FROM ubuntu:jammy
LABEL NAME fred78290/vscode-server

ENV VSCODE_SERVER_HOME_DIR=/home/vscode-server
ENV VSCODE_SERVER_DATA_DIR=/usr/share/vscode-server
ENV TINI_VERSION v0.19.0
ENV DOCKER_VERSION 23.0.3

ENV NODE_VERSION 19.9.0
ENV YARN_VERSION 1.22.19
ENV GOLANG_VERSION 1.20.3
ENV PATH /usr/local/go/bin:/usr/local/yarn/bin:$PATH

EXPOSE 8000

ADD docker-entrypoint.sh /docker-entrypoint.sh
ADD prepare.sh /prepare.sh

RUN /prepare.sh && rm /prepare.sh

WORKDIR ${VSCODE_SERVER_HOME_DIR}

USER vscode-server

ENTRYPOINT ["/usr/local/bin/tini", "--"]
CMD [ "/docker-entrypoint.sh" ]