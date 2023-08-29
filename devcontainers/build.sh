#!/bin/bash
set -o pipefail -o nounset
set -e
: "${VSCODE_SERVER_REGISTRY:?Variable not set or empty}"

DEV_VSCODE_SERVER_REGISTRY=${DEV_VSCODE_SERVER_REGISTRY:=${VSCODE_SERVER_REGISTRY}}
PLATFORM=linux/amd64

VERSION=${IMAGE_TAG:=v0.1.0}

pushd $(dirname $0)

cp .devcontainer/local-features/vscode-server/install.sh .devcontainer/local-features/vscode-server/insiders
cp .devcontainer/local-features/vscode-server/install.sh .devcontainer/local-features/vscode-server/stable

devcontainer build --platform ${PLATFORM} --image-name ${DEV_VSCODE_SERVER_REGISTRY}/vscode-insiders:${VERSION} --push --config .devcontainer/config-vscode-insiders.json --workspace-folder .
devcontainer build --platform ${PLATFORM} --image-name ${DEV_VSCODE_SERVER_REGISTRY}/code-server:${VERSION} --push --config .devcontainer/config-code-server.json --workspace-folder .
devcontainer build --platform ${PLATFORM} --image-name ${DEV_VSCODE_SERVER_REGISTRY}/vscode-server-stable:${VERSION} --push --config .devcontainer/config-vscode-server-stable.json --workspace-folder .
devcontainer build --platform ${PLATFORM} --image-name ${DEV_VSCODE_SERVER_REGISTRY}/vscode-server-insiders:${VERSION} --push --config .devcontainer/config-vscode-server-insiders.json --workspace-folder .

rm .devcontainer/local-features/vscode-server/insiders/install.sh
rm .devcontainer/local-features/vscode-server/stable/install.sh

popd