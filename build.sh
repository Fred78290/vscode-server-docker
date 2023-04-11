#!/bin/bash
set -o pipefail -o nounset

: "${VSCODE_SERVER_REGISTRY:?Variable not set or empty}"

IMAGE=$VSCODE_SERVER_REGISTRY/vscode-server:v0.0.0

docker rmi ${IMAGE}
docker build --no-cache -t ${IMAGE} .