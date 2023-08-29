#!/bin/bash

USERNAME="${USERNAME:-"${_REMOTE_USER:-"automatic"}"}"

dpkgArch="$(dpkg --print-architecture)"

case "${dpkgArch##*-}" in
	amd64)
		ARCH='x64'
		GO_ARCH="amd64"
		DOCKER_ARCH=x86_64
		;;
	arm64)
		ARCH='arm64'
		GO_ARCH="arm64"
		DOCKER_ARCH=aarch64
		;;
	*)
		echo "unsupported architecture"
		exit 1
		;;
esac

apt update
apt dist-upgrade -y
apt install tini nano sudo python3 jq gettext-base wget curl git build-essential openssh-client iproute2 libsecret-1-0 dbus-user-session gnome-keyring ca-certificates zlib1g -y --no-install-recommends

curl -fsSL https://code-server.dev/install.sh | sh

cat <<'EOF' > /usr/local/bin/vscode.sh
#!/bin/bash

TRANSPORT_IF=$(ip route get 1 | awk '{print $5;exit}')
LOCAL_IPADDR=$(ip addr show ${TRANSPORT_IF} | grep -m 1 "inet\s" | tr '/' ' ' | awk '{print $2}')

VSCODE_SERVER_NAME=${VSCODE_SERVER_NAME:=${HOSTNAME}}
VSCODE_SERVER_TOKEN=${VSCODE_SERVER_TOKEN:=}
VSCODE_SERVER_LOG=${VSCODE_SERVER_LOG:=info}
VSCODE_SERVER_BIND_ADDR=${VSCODE_SERVER_BIND_ADDR:${LOCAL_IPADDR}}
VSCODE_SERVER_BIND_PORT=${VSCODE_SERVER_BIND_PORT:=8000}
VSCODE_SERVER_DATA_DIR="${VSCODE_SERVER_DATA_DIR:=${HOME}/.vscode-remote}"
VSCODE_SERVER_INSTALL_EXTENSION="${VSCODE_SERVER_INSTALL_EXTENSION:=GitHub.codespaces GitHub.github-vscode-theme GitHub.vscode-pull-request-github github.vscode-github-actions}"
VSCODE_SERVER_EXTRA_ARGS=${VSCODE_SERVER_EXTRA_ARGS:=}

EXT_ARGS=

for EXT in ${VSCODE_SERVER_INSTALL_EXTENSION}
do
	EXT="--install-extension ${EXT}"

	if [ -n "${EXT_ARGS}" ]; then
		EXT_ARGS="${EXT_ARGS} ${EXT}" 
	else
		EXT_ARGS="${EXT}" 
	fi
done

set -o pipefail -o nounset

: "${VSCODE_KEYRING_PASS:?Variable not set or empty}"

ARGS="--bind-addr ${VSCODE_SERVER_BIND_ADDR}:${VSCODE_SERVER_BIND_PORT} \
	--user-data-dir=/workspaces \
	--app-name ${VSCODE_SERVER_NAME}
	--extensions-dir=${VSCODE_SERVER_DATA_DIR} \
	${EXT_ARGS} \
	${VSCODE_SERVER_EXTRA_ARGS}"

if [ -n "${VSCODE_SERVER_TOKEN}" ]; then
	ARGS="${ARGS} --auth password --hashed-password ${VSCODE_SERVER_TOKEN}"
else
	ARGS="${ARGS} --auth none"
fi

ARGS=$(echo -n ${ARGS} | tr -d '\r')

exec dbus-run-session -- sh -c "(echo $VSCODE_KEYRING_PASS | gnome-keyring-daemon --unlock) && code-server ${ARGS}"

EOF

mkdir /workspaces

chown ${USERNAME}:${USERNAME} /workspaces
chmod +x /usr/local/bin/vscode.sh

apt autoclean
apt-get clean -y
rm -rf  /var/lib/apt/lists/*
