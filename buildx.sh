#!/bin/bash
set -o pipefail -o nounset

: "${VSCODE_SERVER_REGISTRY:?Variable not set or empty}"

DEV_VSCODE_SERVER_REGISTRY=${DEV_VSCODE_SERVER_REGISTRY:=${VSCODE_SERVER_REGISTRY}}

VERSION=${IMAGE_TAG:=v0.1.0}
IMAGE=${DEV_VSCODE_SERVER_REGISTRY}/vscode-server

docker buildx build --pull --platform linux/amd64,linux/arm64 \
    --push \
    -t ${IMAGE}:${VERSION} \
    -t ${IMAGE}:latest \
    .
