FROM ubuntu:jammy
LABEL NAME fred78290/vscode-server

ENV VSCODE_SERVER_DATA_DIR=/usr/share/vscode-server
ENV TINI_VERSION v0.19.0

EXPOSE 8080

RUN export DEBIAN_FRONTEND=noninteractive ; apt update; \
	apt dist-upgrade -y; \
	apt install sudo nano gettext-base wget curl build-essential openssh-client nginx iproute2 -y; \
	apt autoclean ; \
	apt-get clean ; \
	rm -rf  /usr/share/doc /usr/share/doc-base

ADD docker-entrypoint.d /docker-entrypoint.d
ADD docker-entrypoint.sh /docker-entrypoint.sh
ADD nginx/* /etc/nginx

RUN [ "$(uname -p)" = "x86_64" ] && ARCH=amd64 || ARCH=arm64; wget https://github.com/krallin/tini/releases/download/${TINI_VERSION}/tini-${ARCH} -O /usr/local/bin/tini

RUN groupadd -g 1000 vscode-server && \
    adduser --uid 1000 --gid 1000 --home /usr/share/vscode-server vscode-server && \
    adduser vscode-server root && \
    chown -R 1000:1000 /usr/share/vscode-server && \
	chmod +x /usr/local/bin/tini /docker-entrypoint.sh && \
	echo 'vscode-server ALL=(ALL) NOPASSWD:ALL' > /etc/sudoers.d/vscode-server

WORKDIR /usr/share/vscode-server

RUN wget -O- https://aka.ms/install-vscode-server/setup.sh | sh

USER vscode-server

ENTRYPOINT ["/usr/local/bin/tini", "--"]
CMD [ "/docker-entrypoint.sh" ]