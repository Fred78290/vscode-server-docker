#!/bin/bash
set -o pipefail -o nounset

: "${VSCODE_SERVER_REGISTRY:?Variable not set or empty}"

DEV_VSCODE_SERVER_REGISTRY=${DEV_VSCODE_SERVER_REGISTRY:=${VSCODE_SERVER_REGISTRY}}

IMAGE=${DEV_VSCODE_SERVER_REGISTRY}/vscode-server:v0.1.0

docker rmi ${IMAGE}
docker build --no-cache -t ${IMAGE} .