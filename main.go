package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func fetchLogs(ctx context.Context, client *http.Client, authToken, nextToken, region, app string) ([]byte, error) {
	u := url.URL{
		Scheme: "https",
		Host:   "api.fly.io",
		Path:   fmt.Sprintf("/api/v1/apps/%s/logs", app),
	}

	q := u.Query()
	q.Set("next_token", nextToken)

	if region != "" {
		q.Set("region", region)
	}

	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", authToken)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}

	return io.ReadAll(resp.Body)
}

func run(ctx context.Context) error {
	var (
		app    = flag.String("app", "", "Fly app name (required)")
		region = flag.String("region", "", "optional region filter (e.g. syd)")
		days   = flag.Int("days", 1, "how many days back to fetch")
	)
	flag.Parse()

	if *app == "" {
		return fmt.Errorf("--app is required")
	}

	if *days < 1 || *days > 7 {
		return fmt.Errorf("--days must be between 1 and 7")
	}

	authToken, err := GetToken(*app, 1*time.Hour)
	if err != nil {
		return fmt.Errorf("get token: %w", err)
	}

	client := &http.Client{
		Timeout:   30 * time.Second,
		Transport: &http.Transport{Proxy: http.ProxyFromEnvironment},
	}

	startTime := time.Now().Add(-time.Duration(*days) * 24 * time.Hour)
	nextToken := strconv.FormatInt(startTime.UnixNano(), 10)

	output, err := os.Create("logs.txt")
	if err != nil {
		return fmt.Errorf("create logs.txt: %w", err)
	}
	defer output.Close()

	for {
		body, err := fetchLogs(ctx, client, authToken, nextToken, *region, *app)
		if err != nil {
			return fmt.Errorf("fetch logs: %w", err)
		}

		var page FlyLog
		if err := json.Unmarshal(body, &page); err != nil {
			return fmt.Errorf("decode logs response: %w", err)
		}

		if err := WriteCompactLog(output, page); err != nil {
			return fmt.Errorf("write log: %w", err)
		}

		if page.Meta.NextToken == "" {
			break
		}

		nextToken = page.Meta.NextToken
	}

	// Flush and catch write/close errors before finishing
	if err := output.Close(); err != nil {
		return fmt.Errorf("close output file: %w", err)
	}
	return nil
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
