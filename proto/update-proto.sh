#!/bin/bash
set -e

# Generate Go protobuf code
protoc --go_out=. --go_opt=paths=source_relative proto/messages.proto

# Generate JavaScript protobuf code
npx -y pbjs -t static-module -w closure -o proto/messages_pb.js proto/messages.proto
