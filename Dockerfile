ARG GO_VERSION=1.23.0
FROM golang:${GO_VERSION}-bookworm AS builder

# dependencies layer
ENV CGO_ENABLED=0 GOOS=linux GOARCH=amd64
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download && go mod verify

# frontend build
RUN apt-get update && apt-get install -y --no-install-recommends nodejs npm && rm -rf /var/lib/apt/lists/*
COPY frontend/package*.json ./frontend/
RUN cd frontend && npm ci
COPY frontend ./frontend
ARG MODE=production
RUN cd frontend && npm run build:${MODE}
RUN mkdir -p backend/dist

# build stage
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
