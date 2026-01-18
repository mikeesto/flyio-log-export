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
		app        = flag.String("app", "", "Fly app name (required)")
		region     = flag.String("region", "", "optional region filter (e.g. syd)")
		days       = flag.Int("days", 1, "how many days back to fetch")
		idle       = flag.Duration("idle", 10*time.Second, "stop if no new logs for this long AND caught up")
		catchup    = flag.Duration("catchup", 2*time.Minute, "consider 'caught up' if newest log is within this of now")
		compactMax = flag.Int("compact-max", 0, "max message chars in compact output (0 = no limit)")
		output     = flag.String("out", "logs.compact.txt", "output file path (use - for stdout)")
	)
	flag.Parse()

	if *app == "" {
		fatalf("--app is required")
	}

	if *days < 1 || *days > 15 {
		fatalf("--days must be between 1 and 15")
	}

	token, err := GetToken(*app, 1*time.Hour)
	must(err)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// --- Setup HTTP Request ---
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

	client := &http.Client{Transport: &http.Transport{Proxy: http.ProxyFromEnvironment}}
	resp, err := client.Do(req)
	must(err)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
		fatalf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}

	// --- Setup Output ---
	var out io.Writer
	var closer io.Closer

	if *output == "-" {
		out = os.Stdout
		closer = io.NopCloser(nil)
	} else {
		f, err := os.Create(*output)
		must(err)
		out = f
		closer = f
		fmt.Fprintf(os.Stderr, "writing compact logs to %s\n", *output)
	}

	bufOut := bufio.NewWriter(out)

	finish := func() {
		bufOut.Flush()
		closer.Close()
	}
	defer finish()

	// --- Start Dedicated Reader Routine ---
	linesCh := make(chan string, 100)
	errCh := make(chan error, 1)

	go func() {
		scanner := bufio.NewScanner(resp.Body)
		// Increase buffer size for huge log lines
		scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
		for scanner.Scan() {
			linesCh <- scanner.Text()
		}
		if err := scanner.Err(); err != nil {
			errCh <- err
		}
		close(linesCh)
	}()

	// --- Main Loop ---
	var newest time.Time
	timer := time.NewTimer(*idle)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			fmt.Fprintln(os.Stderr, "\nstopped:", ctx.Err())
			return

		case <-timer.C:
			// Idle timeout hit
			if newest.IsZero() {
				// Haven't seen a timestamp yet, keep waiting
				timer.Reset(*idle)
				continue
			}
			if time.Since(newest) <= *catchup {
				fmt.Fprintf(os.Stderr, "\ncaught up (newest log %s), stopping\n", newest.UTC().Format(time.RFC3339))
				return
			}
			// Not caught up yet
			timer.Reset(*idle)

		case err := <-errCh:
			finish()
			fatalf("stream error: %v", err)

		case line, ok := <-linesCh:
			if !ok {
				fmt.Fprintln(os.Stderr, "stream ended by server")
				return
			}

			// 1. Parse JSON once
			var fl FlyLog
			if err := json.Unmarshal([]byte(line), &fl); err != nil {
				// If parsing fails, print raw line
				fmt.Fprintf(bufOut, "????-??-??T??:??:??Z ????? ???? ???????? raw: %s\n", line)
				continue
			}

			// 2. Update newest timestamp from the struct
			if ts := extractTimeFromStruct(fl); !ts.IsZero() && ts.After(newest) {
				newest = ts
			}

			// 3. Write using the struct
			WriteCompactLog(bufOut, fl, *compactMax)

			// Reset idle timer
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

func extractTimeFromStruct(fl FlyLog) time.Time {
	for _, data := range fl.Data {
		s := data.Attributes.Timestamp
		if s == "" {
			continue
		}
		if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
			return t
		}
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			return t
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
