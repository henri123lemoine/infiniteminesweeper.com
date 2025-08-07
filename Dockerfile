ARG PYTHON_VERSION=3.11
ARG NODE_VERSION=22
ARG GO_VERSION=1.23.0

# 0. Python stage just for sprite gen
FROM python:${PYTHON_VERSION}-slim AS sprite-gen
WORKDIR /gen

COPY scripts/python ./scripts/python
COPY frontend/assets/raw ./frontend/assets/raw
COPY frontend/assets/sprites.yaml ./frontend/assets/sprites.yaml
RUN pip install --no-cache-dir pillow numpy PyYAML pathlib \
 && python3 scripts/python/sprite_sheet_gen.py \
      frontend/assets/raw \
      frontend/assets/sprites.yaml \
      ./spritesheet.png \
      ./spritesheet.json

# 1. Node builder for React app
FROM node:${NODE_VERSION}-bookworm AS frontend
WORKDIR /app
COPY frontend/package*.json ./
RUN npm ci
COPY frontend/ ./

RUN mkdir -p src/assets

# pull in just the generated sprite assets
COPY --from=sprite-gen /gen/spritesheet.png  src/assets/spritesheet.png
COPY --from=sprite-gen /gen/spritesheet.json src/assets/spritesheet.json
RUN npm run build:production

# 2. Go builder
FROM golang:${GO_VERSION}-bookworm AS gobuild
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download && go mod verify
COPY backend ./backend
COPY --from=frontend /backend/dist ./backend/dist
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" -o /run-app ./backend

# 3. Runtime
FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates \
 && rm -rf /var/lib/apt/lists/*
COPY --from=gobuild /run-app /usr/local/bin/
VOLUME ["/data"]
ENV PORT=8080
EXPOSE 8080
CMD ["run-app"]
