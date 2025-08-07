# Build arguments
ARG PYTHON_VERSION=3.11
ARG NODE_VERSION=22
ARG GO_VERSION=1.23.0
ARG MODE=production

# Stage 0: Sprite generator
FROM python:${PYTHON_VERSION}-slim AS sprite-gen
WORKDIR /gen

COPY scripts/python ./scripts/python
COPY frontend/assets/raw ./frontend/assets/raw
COPY frontend/assets/sprites.yaml ./frontend/assets/sprites.yaml

RUN pip install --no-cache-dir pillow numpy PyYAML pathlib && \
    python3 scripts/python/sprite_sheet_gen.py \
        frontend/assets/raw \
        frontend/assets/sprites.yaml \
        ./spritesheet.png \
        ./spritesheet.json

# Stage 1: Frontend build
FROM node:${NODE_VERSION}-bookworm AS frontend
WORKDIR /app

# Install dependencies first (better caching)
COPY frontend/package*.json ./
RUN npm ci

# Copy source and proto
COPY frontend ./
RUN mkdir -p src/assets src/gen
COPY proto/messages.proto ./proto/messages.proto

# Copy generated sprite assets
COPY --from=sprite-gen /gen/spritesheet.png ./src/assets/spritesheet.png
COPY --from=sprite-gen /gen/spritesheet.json ./src/assets/spritesheet.json

# Generate JavaScript protobuf stubs
RUN npx --no-install pbjs \
      -t static-module -w es6 \
      -o src/gen/messages_pb.js \
      proto/messages.proto
RUN ls -l src/gen && head -5 src/gen/messages_pb.js

# Set production environment
ENV NODE_ENV=production

# Build based on MODE
ARG MODE=production
RUN if [ "$MODE" = "production" ]; then \
      npm run build:production; \
    else \
      npm run build:development; \
    fi

# Stage 2: Backend build
FROM golang:${GO_VERSION}-bookworm AS gobuild
WORKDIR /src

# Install protobuf tools
RUN apt-get update && \
    apt-get install -y --no-install-recommends protobuf-compiler && \
    go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.34.0 && \
    rm -rf /var/lib/apt/lists/*

# Download Go modules first
COPY go.mod go.sum ./
RUN go mod download && go mod verify

# Copy backend sources
COPY backend ./backend

# Copy React bundle from frontend stage
COPY --from=frontend /backend/dist ./backend/dist

# Generate Go protobuf stubs
COPY proto/messages.proto ./proto/messages.proto
RUN mkdir -p backend/gen && \
    protoc --go_out=backend/gen --go_opt=paths=source_relative proto/messages.proto

# Build static binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" -o /run-app ./backend

# Stage 3: Runtime
FROM debian:bookworm-slim

RUN apt-get update && \
    apt-get install -y --no-install-recommends ca-certificates && \
    rm -rf /var/lib/apt/lists/*

COPY --from=gobuild /run-app /usr/local/bin/

VOLUME ["/data"]
ENV PORT=8080
EXPOSE 8080
CMD ["run-app"]
