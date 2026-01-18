# Fly.io Log Export

A lightweight Go utility that extracts Fly.io application logs into a compact, human-readable format.

It connects to the Fly API, backfills a specific time window, streams until it catches up to the present, and then exits.

## Motivation

I built this to get a clean, static slice of recent logs.

While `fly logs` is great for live tailing, I often want to collate logs from the past week or two into a format that is:

1.  **Readable** (No raw JSON, no ANSI color codes).
2.  **Finite** (It stops writing once it catches up).
3.  **Token-efficient** (Ideal for pasting into LLMs with large context windows, such as Gemini, for debugging).

## Auth

In order to access the Fly.io logs API, you need to authenticate. This tool supports two methods:

- **flyctl Auth:** If you have the `flyctl` CLI installed and authenticated, the tool can automatically use it for credentials. This is the default/easiest method.
- **Manual Token:** Set the `FLY_API_TOKEN` environment variable with your Fly.io API token.

## Usage

### Prerequisites

- Go 1.22+
- `flyctl` installed (optional, for automatic authentication)

### Quick Start

Export the last 24 hours of logs to a file:

```bash
go run . --app my-app-name
```

Fetch the last 3 days from a specific region and pipe to another tool:

```bash
go run . --app my-app-name --days 3 --region syd --out - | grep "error"
```

### Flags

| Flag            | Default            | Description                                                        |
| :-------------- | :----------------- | :----------------------------------------------------------------- |
| `--app`         | _required_         | The name of your Fly application.                                  |
| `--days`        | `1`                | How many days back to start fetching logs (max 15).                |
| `--region`      | _(all)_            | Filter logs by region (e.g., `syd`, `iad`).                        |
| `--out`         | `logs.compact.txt` | Output path. Use `-` for stdout.                                   |
| `--compact-max` | `0`                | Truncate messages longer than N characters (0 = no limit).         |
| `--idle`        | `10s`              | How long to wait for new logs before checking if we are caught up. |

## Output Format

The tool produces a space-separated format designed for easy scanning:

```text
TIMESTAMP            LVL   REG  INSTANCE MESSAGE
2025-01-20T08:15:23Z INFO  syd  9185936b Request processed in 45ms status=200
2025-01-20T08:15:24Z ERROR syd  9185936b Database connection failed \n retrying...
```

## Note on API Usage

This tool utilises the "undocumented" [internal Fly.io HTTP API endpoint](https://fly.io/docs/monitoring/logs-api-options/#1-http-api-same-as-fly-logs) to stream logs. This gives you access to historical logs going back to the current retention window (about 15 days).
