#!/bin/bash
set -o pipefail -o nounset

: "${VSCODE_SERVER_REGISTRY:?Variable not set or empty}"

DEV_VSCODE_SERVER_REGISTRY=${DEV_VSCODE_SERVER_REGISTRY:=${VSCODE_SERVER_REGISTRY}}
PLATFORM=linux/amd64,linux/arm64
#PLATFORM=linux/amd64

VERSION=${IMAGE_TAG:=v0.1.0}
IMAGE=${DEV_VSCODE_SERVER_REGISTRY}/vscode-server:${VERSION}

pushd $(dirname $0)
devcontainer build --platform ${PLATFORM} --image-name ${IMAGE} --push --workspace-folder .
popd