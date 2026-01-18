package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

func main() {
	var (
		app     = flag.String("app", "", "Fly app name (required)")
		outPath = flag.String("out", "logs.jsonl", "output JSONL path")
		region  = flag.String("region", "", "optional region filter (e.g. syd)")
		days    = flag.Int("days", 1, "how many days back to fetch")
		idle    = flag.Duration("idle", 10*time.Second, "stop if no new logs for this long AND caught up")
		catchup = flag.Duration("catchup", 2*time.Minute, "consider 'caught up' if newest log is within this of now")
	)
	flag.Parse()

	if *app == "" {
		fatalf("--app is required")
	}
	token, err := GetToken(*app, 1*time.Hour)
	must(err)

	// Cancel on Ctrl+C
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	startTime := time.Now().UTC().Add(-time.Duration(*days) * 24 * time.Hour).Format(time.RFC3339)

	u := url.URL{
		Scheme: "https",
		Host:   "api.fly.io",
		Path:   fmt.Sprintf("/api/v1/apps/%s/logs", *app),
	}
	q := u.Query()
	q.Set("start_time", startTime)
	if *region != "" {
		q.Set("region", *region)
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	must(err)
	req.Header.Set("Authorization", token)

	// Streaming response; don’t set a short client timeout.
	client := &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
		},
	}

	resp, err := client.Do(req)
	must(err)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
		fatalf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}

	out, err := os.Create(*outPath)
	must(err)
	defer out.Close()

	// We’ll read lines ourselves so we can implement an idle timeout cleanly.
	reader := bufio.NewReader(resp.Body)

	// Tracks newest timestamp seen (if present in JSON).
	var newest time.Time

	// Idle timer: if it fires and we’re “caught up”, we stop.
	timer := time.NewTimer(*idle)
	defer timer.Stop()

	for {
		// Read one JSONL line (blocking). We do it in a goroutine so we can select on idle/cancel.
		type readResult struct {
			line []byte
			err  error
		}
		ch := make(chan readResult, 1)
		go func() {
			line, err := reader.ReadBytes('\n')
			// If stream ends without trailing newline, still return the bytes.
			if err == io.EOF && len(line) > 0 {
				err = nil
			}
			ch <- readResult{line: line, err: err}
		}()

		select {
		case <-ctx.Done():
			fmt.Fprintln(os.Stderr, "stopped:", ctx.Err())
			return

		case <-timer.C:
			// No new lines for `idle`. Stop only if we appear caught up.
			if newest.IsZero() {
				// If we couldn't parse timestamps, be conservative and keep waiting.
				timer.Reset(*idle)
				continue
			}
			if time.Since(newest) <= *catchup {
				fmt.Fprintf(os.Stderr, "caught up (newest log %s), stopping\n", newest.UTC().Format(time.RFC3339))
				return
			}
			// Not caught up yet; keep waiting.
			timer.Reset(*idle)

		case rr := <-ch:
			if rr.err != nil {
				if rr.err == io.EOF {
					// Server closed stream; we’re done.
					fmt.Fprintln(os.Stderr, "stream ended by server")
					return
				}
				fatalf("read error: %v", rr.err)
			}
			line := strings.TrimSpace(string(rr.line))
			if line == "" {
				timer.Reset(*idle)
				continue
			}

			// Write raw JSON line to output (preserve exact payload).
			_, err := out.WriteString(line + "\n")
			must(err)

			// Update newest timestamp if the JSON has something usable.
			if ts := extractTimestamp(line); !ts.IsZero() && ts.After(newest) {
				newest = ts
			}

			// Reset idle timer whenever we get a line.
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(*idle)
		}
	}
}

func extractTimestamp(jsonLine string) time.Time {
	// Fly’s logs commonly include `timestamp` (and sometimes `time`).
	var m map[string]any
	if err := json.Unmarshal([]byte(jsonLine), &m); err != nil {
		return time.Time{}
	}
	for _, key := range []string{"timestamp", "time"} {
		if v, ok := m[key]; ok {
			if s, ok := v.(string); ok {
				// Most log timestamps are RFC3339-ish.
				if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
					return t
				}
				if t, err := time.Parse(time.RFC3339, s); err == nil {
					return t
				}
			}
		}
	}
	return time.Time{}
}

func must(err error) {
	if err != nil {
		fatalf("%v", err)
	}
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
