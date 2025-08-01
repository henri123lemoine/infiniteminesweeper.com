ARG GO_VERSION=1.23
FROM golang:${GO_VERSION}-bookworm AS builder

# dependencies layer
ENV CGO_ENABLED=0 GOOS=linux GOARCH=amd64
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download && go mod verify

COPY backend ./backend
RUN go build -trimpath -ldflags="-s -w" -o /run-app ./backend

# runtime image
FROM debian:bookworm-slim

RUN apt-get update && \
    apt-get install -y --no-install-recommends ca-certificates && \
    rm -rf /var/lib/apt/lists/*

# Persist snapshots/WAL under /data
VOLUME ["/data"]
COPY --from=builder /run-app /usr/local/bin/

CMD ["run-app"]
