# flyio-log-export

`fly logs` is useful for viewing live logs, but it does not make it easy to download historical logs. So I made this tool to download every available log for a specified period (up to 7 days) and write them into a `logs.txt` file.

## Requirements

- Go 1.22 or later
- A Fly.io API token, or an authenticated `fly` CLI installation

## Usage

Export the last 24 hours of logs:

```bash
go run . --app my-app
```

Export the last three days:

```bash
go run . --app my-app --days 3
```

Export logs from one region:

```bash
go run . --app my-app --days 3 --region syd
```

The command creates `logs.txt` in the current directory. If the file already exists, it is replaced.

### Options

| Option     | Default     | Description                             |
| ---------- | ----------- | --------------------------------------- |
| `--app`    | Required    | Fly.io application name                 |
| `--days`   | `1`         | Number of days to request, from 1 to 7  |
| `--region` | All regions | Fly.io region to include, such as `syd` |

## Authentication

If `FLY_API_TOKEN` is set, the tool uses it directly:

```bash
export FLY_API_TOKEN="$(fly auth token)"
go run . --app my-app
```

If `FLY_API_TOKEN` is not set, the tool runs `fly tokens create deploy` to create a one hour deploy token for the requested application. This requires you to have the `fly` CLI installed and authenticated.

## Output

Each log entry is written on one line:

```text
2026-09-25T08:15:23.47022662Z INFO  Request completed status=200
2026-09-25T08:16:04Z ERROR Database connection failed \n retrying
```

Fly.io only keeps recent logs for a limited period, currently around 7 days. This tool can only export logs that Fly.io still has available.
