# VSCode Server in docker

This project is a simple way to embed vscode server in a docker container and serving vscode for remote development.

The environment include, nodejs, npm, golang, php.

## Env variables

Before to build or deploy vscode-server, you need to declare some variables

````
export VSCODE_SERVER_IMAGE=fred78290/vscode-server:v0.1.0
export VSCODE_HOSTNAME=vscode-server.acme.com
````


## Build it

To build locally execute : `build.sh`
To build and publish in your docker registry : `buildx.sh`

## Run in local docker

To run in you local docker : `run.sh`. You need to declare **VSCODE_KEYRING_PASS**

```
export VSCODE_KEYRING_PASS=754A3584-CD8B-4146-9DEB-4FC288F09E2E
````

## Install in Kubernetes

If you expect to use oauth2 authentication : https://oauth2-proxy.github.io/oauth2-proxy/, you need to declare extras variables

```
export VSCODE_OAUTH2_PROXY_PROVIDER=google
export VSCODE_OAUTH2_PROXY_CLIENT_ID=123456789-abcdefgh.apps.googleusercontent.com
export VSCODE_OAUTH2_PROXY_CLIENT_SECRET=7D0585D1-EA9C-4396-864F-8742114DC6C9
````

To deploy in kubernetes cluster : `kubernetes.sh`
