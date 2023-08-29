#!/bin/bash
set -e

echo "Start"

USERNAME="${USERNAME:-"${_REMOTE_USER:-"automatic"}"}"
VSCODE_SERVER_DISTRO=${VSCODE_SERVER_DISTRO:=insider}

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

wget -qO- https://packages.microsoft.com/keys/microsoft.asc | gpg --dearmor > packages.microsoft.gpg
install -D -o root -g root -m 644 packages.microsoft.gpg /etc/apt/keyrings/packages.microsoft.gpg
echo "deb [arch=amd64,arm64,armhf signed-by=/etc/apt/keyrings/packages.microsoft.gpg] https://packages.microsoft.com/repos/code stable main" > /etc/apt/sources.list.d/vscode.list

apt update
apt install apt-transport-https wget gpg uuid-runtime tini nano sudo python3 jq gettext-base wget curl git build-essential openssh-client iproute2 libsecret-1-0 dbus-user-session gnome-keyring ca-certificates zlib1g -y --no-install-recommends
apt install code-insiders -y

apt dist-upgrade -y
apt autoremove -y

rm -f packages.microsoft.gpg

cat <<'EOF' > /usr/local/bin/vscode.sh
#!/bin/bash

TRANSPORT_IF=$(ip route get 1 | awk '{print $5;exit}')
LOCAL_IPADDR=$(ip addr show ${TRANSPORT_IF} | grep -m 1 "inet\s" | tr '/' ' ' | awk '{print $2}')

VSCODE_SERVER_NAME=${VSCODE_SERVER_NAME:=${HOSTNAME}}
VSCODE_SERVER_TOKEN=${VSCODE_SERVER_TOKEN:=}
VSCODE_SERVER_LOG=${VSCODE_SERVER_LOG:=info}
VSCODE_SERVER_BIND_ADDR=${VSCODE_SERVER_BIND_ADDR:=${LOCAL_IPADDR}}
VSCODE_SERVER_BIND_PORT=${VSCODE_SERVER_BIND_PORT:=8000}
VSCODE_SERVER_DATA_DIR="${VSCODE_SERVER_DATA_DIR:=${HOME}/.vscode-remote}"
VSCODE_SERVER_INSTALL_EXTENSION="${VSCODE_SERVER_INSTALL_EXTENSION:=GitHub.codespaces GitHub.github-vscode-theme GitHub.vscode-pull-request-github github.vscode-github-actions}"
VSCODE_SERVER_EXTRA_ARGS=${VSCODE_SERVER_EXTRA_ARGS:=}

export CLOUDENV_ENVIRONMENT_ID=${CLOUDENV_ENVIRONMENT_ID:=$(uuidgen)}
export CODESPACES=${CODESPACES:=true}
export CODESPACE_NAME=${VSCODE_SERVER_NAME}

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

ARGS="serve-web \
	--accept-server-license-terms \
	--host ${VSCODE_SERVER_BIND_ADDR} \
	--port ${VSCODE_SERVER_BIND_PORT} \
	--log ${VSCODE_SERVER_LOG} \
	--server-data-dir '${VSCODE_SERVER_DATA_DIR}' \
	--cli-data-dir '${VSCODE_SERVER_DATA_DIR}/cli' \
	--user-data-dir '${VSCODE_SERVER_DATA_DIR}/data' \
	--extensions-dir '${VSCODE_SERVER_DATA_DIR}/extensions' \
	${VSCODE_SERVER_EXTRA_ARGS}"

if [ -n "${VSCODE_SERVER_TOKEN}" ]; then
	ARGS="${ARGS} --connection-token ${VSCODE_SERVER_TOKEN}"
else
	ARGS="${ARGS} --without-connection-token"
fi

ARGS=$(echo -n ${ARGS} | tr -d '\r')

echo "ARGS=${ARGS}"

if [ -n "${EXT_ARGS}" ]; then
	code-insiders --log ${VSCODE_SERVER_LOG} \
		--user-data-dir "${VSCODE_SERVER_DATA_DIR}/data" \
		--extensions-dir "${VSCODE_SERVER_DATA_DIR}/extensions" \
		${EXT_ARGS} \
		${VSCODE_SERVER_EXTRA_ARGS}
fi

exec dbus-run-session -- sh -c "(echo $VSCODE_KEYRING_PASS | gnome-keyring-daemon --unlock) && code-insiders ${ARGS}"

EOF

mkdir /workspaces

chown ${USERNAME}:${USERNAME} /workspaces
chmod +x /usr/local/bin/vscode.sh

apt autoclean
apt-get clean -y
rm -rf  /var/lib/apt/lists/*
