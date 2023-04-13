#!/bin/bash
set -o pipefail -o nounset

: "${VSCODE_SERVER_REGISTRY:?Variable not set or empty}"

DEV_VSCODE_SERVER_REGISTRY=${DEV_VSCODE_SERVER_REGISTRY:=${VSCODE_SERVER_REGISTRY}}

sudo rm -rf out

VERSION=${IMAGE_TAG:=v0.1.0}
IMAGE=${DEV_VSCODE_SERVER_REGISTRY}/vscode-server-helper

make -e REGISTRY=$VSCODE_SERVER_REGISTRY -e TAG=$VERSION container-push-manifest
