#!/bin/bash

VERSION=v0.1.0
REGISTRY=fred78290
IMAGE=${REGISTRY}/vscode-server

docker buildx build --pull --platform linux/amd64,linux/arm64 --push -t ${IMAGE}:${VERSION} .
