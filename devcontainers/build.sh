#!/bin/bash

devcontainer build --platform linux/amd64 --image-name devregistry.aldunelabs.com/vscode-container:v0.1.0 --push --workspace-folder .
