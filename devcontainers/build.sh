#!/bin/bash
set -o pipefail -o nounset

: "${VSCODE_SERVER_REGISTRY:?Variable not set or empty}"

DEV_VSCODE_SERVER_REGISTRY=${DEV_VSCODE_SERVER_REGISTRY:=${VSCODE_SERVER_REGISTRY}}
PLATFORM=linux/amd64

VERSION=${IMAGE_TAG:=v0.1.0}
VSCODE_SERVER_IMAGE=${DEV_VSCODE_SERVER_REGISTRY}/vscode-server:${VERSION}
CODE_SERVER_IMAGE=${DEV_VSCODE_SERVER_REGISTRY}/code-server:${VERSION}

pushd $(dirname $0)
devcontainer build --platform ${PLATFORM} --image-name ${CODE_SERVER_IMAGE} --push --config .devcontainer/devcontainer-code-server.json --workspace-folder .
devcontainer build --platform ${PLATFORM} --image-name ${VSCODE_SERVER_IMAGE} --push --config .devcontainer/devcontainer-vscode-server.json --workspace-folder .
popd