#!/bin/bash
set -o pipefail -o nounset

: "${VSCODE_SERVER_REGISTRY:?Variable not set or empty}"

VERSION=v0.1.0
IMAGE=${VSCODE_SERVER_REGISTRY}/vscode-server

docker buildx build --pull --platform linux/amd64,linux/arm64 \
    --push \
    -t ${IMAGE}:${VERSION} \
    -t ${IMAGE}:latest \
    .
