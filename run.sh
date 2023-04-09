#!/bin/bash
docker rm -f devcontainer

docker run -p 8000:8000 --name devcontainer \
	--rm \
	devregistry.aldunelabs.com/vscode-server:v0.0.0