#!/bin/bash
docker rm -f devcontainer

docker run -p 8000:8000 --name devcontainer \
	--rm \
	-e VSCODE_KEYRING_PASS=${VSCODE_KEYRING_PASS} \
	-v ${PWD}/docker-entrypoint.sh:/docker-entrypoint.sh \
	devregistry.aldunelabs.com/vscode-server:v0.0.0