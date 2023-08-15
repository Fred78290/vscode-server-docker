#-------------------------------------------------------------------------------------------------------------
# Copyright (c) Microsoft Corporation. All rights reserved.
# Licensed under the MIT License. See https://go.microsoft.com/fwlink/?linkid=2090316 for license information.
#-------------------------------------------------------------------------------------------------------------
ARG BASEIMAGE=mcr.microsoft.com/devcontainers/universal:2-focal
FROM $BASEIMAGE

LABEL NAME fred78290/vscode-server

RUN apt-get update && export DEBIAN_FRONTEND=noninteractive \
    && apt-get upgrade -y \
    && rm -rf /var/lib/apt/lists/*

# Default to bash shell (other shells available at /usr/bin/fish and /usr/bin/zsh)
ENV SHELL=/bin/bash
ENV DOCKER_BUILDKIT=1
ENV VSCODE_USER=codespace
ENV VSCODE_RUNNING_USER=codespace
ENV USER_ID=1000
ENV GROUP_ID=1000

ADD docker-entrypoint.d /docker-entrypoint.d
ADD docker-entrypoint.sh /docker-entrypoint.sh

VOLUME [ "/var/lib/docker", "/workspaces" ]

EXPOSE 8000 2222

# Fire Docker/Moby script if needed
ENTRYPOINT ["/usr/bin/tini", "--" ]
CMD [ "/usr/local/share/docker-init.sh", "/usr/local/share/ssh-init.sh", "/docker-entrypoint.sh", "sleep", "infinity" ]