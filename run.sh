#!/bin/bash

docker run -p 8080:8080 \
	-v $PWD/nginx.conf:/etc/nginx/sites-available/default \
	--rm \
	devregistry.aldunelabs.com/vscode-server:v0.0.0