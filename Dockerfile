ARG BASEIMAGE=fred78290/vscode-container:v0.1.0

FROM $BASEIMAGE
LABEL NAME fred78290/vscode-server

ENV VSCODE_USER=codespace
ENV VSCODE_RUNNING_USER=codespace
ENV USER_ID=1000
ENV GROUP_ID=1000

VOLUME /workspaces

EXPOSE 8000 2222

ADD docker-entrypoint.d /docker-entrypoint.d
ADD docker-entrypoint.sh /docker-entrypoint.sh
ADD prepare.sh /prepare.sh

RUN /prepare.sh && rm /prepare.sh

USER $USER_ID:$GROUP_ID

ENTRYPOINT ["/usr/bin/tini", "--" ]
CMD [ "/usr/local/share/docker-init.sh", "/usr/local/share/ssh-init.sh", "/docker-entrypoint.sh", "sleep", "infinity" ]