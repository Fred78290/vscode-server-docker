FROM ubuntu:jammy
LABEL NAME fred78290/vscode-server

ENV VSCODE_SERVER_HOME_DIR=/home/vscode-server
ENV VSCODE_SERVER_DATA_DIR=/usr/share/vscode-server
ENV TINI_VERSION v0.19.0
ENV VSCODE_KEYRING_PASS=

EXPOSE 8000

RUN export DEBIAN_FRONTEND=noninteractive ; apt update; \
	apt dist-upgrade -y; \
	apt install nano sudo gettext-base node wget curl git build-essential openssh-client iproute2 libsecret-1-0 dbus-user-session gnome-keyring ca-certificates -y --no-install-recommends; \
	apt autoclean ; \
	apt-get clean -y ; \
	echo 'vscode-server ALL=(ALL) NOPASSWD:ALL' > /etc/sudoers.d/vscode-server ; \
	rm -rf  /usr/share/doc /usr/share/doc-base /var/lib/apt/lists/*

ADD docker-entrypoint.sh /docker-entrypoint.sh

RUN [ "$(uname -p)" = "x86_64" ] && ARCH=amd64 || ARCH=arm64; wget https://github.com/krallin/tini/releases/download/${TINI_VERSION}/tini-${ARCH} -O /usr/local/bin/tini

RUN groupadd -g 1000 vscode-server \
    && adduser --uid 1000 --gid 1000 --home ${VSCODE_SERVER_HOME_DIR} vscode-server \
    && adduser vscode-server root \
	&& mkdir -p ${VSCODE_SERVER_DATA_DIR} \
    && chown -R vscode-server:vscode-server ${VSCODE_SERVER_DATA_DIR} \
	&& chmod +x /usr/local/bin/tini /docker-entrypoint.sh

WORKDIR ${VSCODE_SERVER_HOME_DIR}

RUN wget -O- https://aka.ms/install-vscode-server/setup.sh | sh

USER vscode-server

ENTRYPOINT ["/usr/local/bin/tini", "--"]
CMD [ "/docker-entrypoint.sh" ]