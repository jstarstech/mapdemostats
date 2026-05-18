# mapdemostats

Small Go service that replays demo map statistics from `data/demoData.csv` into Redis.

The app reads timestamped CSV rows, keeps the first report for each hour, and publishes each selected row every 2 seconds to the Redis Pub/Sub channel `hub-counts`. After reaching the end of the data, it loops forever.

## Requirements

- Go 1.26+
- Redis
- Docker, optional

## Configuration

| Variable | Default | Description |
| --- | --- | --- |
| `DATA_FILE` | `data/demoData.csv` | CSV file to replay |
| `REDIS_URL` | `redis://localhost:6379` | Redis connection URL |
| `REDIS_CHANNEL` | `hub-counts` | Redis Pub/Sub channel |
| `REDIS_CONNECT_RETRY` | `true` | Keep retrying Redis startup connection forever |
| `REDIS_CONNECT_RETRY_INTERVAL` | `2s` | Delay between Redis startup connection attempts |
| `GROUP_BY_HOUR` | `true` | Publish only the first report for each hour |
| `BULK_PUBLISH` | `true` | Publish the full CSV row instead of individual values |
| `PUBLISH_INTERVAL` | `2s` | Delay between published reports |

Variables can be exported in the shell or placed in a local `.env` file:

```env
DATA_FILE=data/demoData.csv
REDIS_URL=redis://localhost:6379
REDIS_CHANNEL=hub-counts
REDIS_CONNECT_RETRY=true
REDIS_CONNECT_RETRY_INTERVAL=2s
GROUP_BY_HOUR=true
BULK_PUBLISH=true
PUBLISH_INTERVAL=2s
```

## Run

```sh
go run .
```

With a custom Redis URL:

```sh
REDIS_URL=redis://localhost:6379 go run .
```

## Docker

```sh
docker build -t mapdemostats .
docker run --rm --network host mapdemostats
```

The image includes `data/demoData.csv` as the default data file. To use a custom CSV, mount it and point `DATA_FILE` at the mounted path:

```sh
docker run --rm --network host \
  -v "$PWD/data/custom.csv:/data/custom.csv:ro" \
  -e DATA_FILE=/data/custom.csv \
  mapdemostats
```

## Test

```sh
go test ./...
```
