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
| `REDIS_URL` | `redis://localhost:6379` | Redis connection URL |

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

## Test

```sh
go test ./...
```
