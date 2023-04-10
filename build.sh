#!/bin/bash
IMAGE=devregistry.aldunelabs.com/vscode-server:v0.0.0

docker rmi ${IMAGE}
docker build --no-cache -t ${IMAGE} .