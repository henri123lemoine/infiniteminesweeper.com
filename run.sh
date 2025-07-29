#!/bin/bash

set -e

npm run build

if [[ "$1" == "fast" ]]; then
  go run .
else
  docker build -t infiniteminesweeper .
  docker run --env-file .env -p 8080:8080 infiniteminesweeper
fi
