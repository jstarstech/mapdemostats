# syntax=docker/dockerfile:1

FROM golang:1.20 AS build-stage

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY *.go ./

RUN CGO_ENABLED=0 GOOS=linux go build -o /mapdemostats

FROM gcr.io/distroless/base-debian11 AS build-release-stage

WORKDIR /

COPY --from=build-stage /mapdemostats /mapdemostats
COPY data/demoData.csv /data/demoData.csv

USER nonroot:nonroot

VOLUME /data

ENTRYPOINT ["/mapdemostats"]
