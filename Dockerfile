# syntax=docker/dockerfile:1.7

FROM golang:1.26 AS build-stage

WORKDIR /app

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go mod download

COPY *.go ./
COPY data ./data

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /mapdemostats

FROM gcr.io/distroless/static-debian13 AS build-release-stage

WORKDIR /

COPY --from=build-stage /mapdemostats /mapdemostats
COPY data/demoData.csv /data/demoData.csv

USER nonroot:nonroot

ENTRYPOINT ["/mapdemostats"]
