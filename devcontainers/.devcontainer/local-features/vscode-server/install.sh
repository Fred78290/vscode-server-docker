#!/bin/bash
set -e

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

apt update
apt dist-upgrade -y
apt install tini nano sudo python3 jq gettext-base wget curl git build-essential openssh-client iproute2 libsecret-1-0 dbus-user-session gnome-keyring ca-certificates zlib1g -y --no-install-recommends

# https://github.com/microsoft/vscode/issues/135856#issuecomment-1170091265
LATEST="$(curl -fsSL https://update.code.visualstudio.com/api/commits/${VSCODE_SERVER_DISTRO}/server-linux-x64-web | jq -r 'first')"
wget https://az764295.vo.msecnd.net/${VSCODE_SERVER_DISTRO}/${LATEST}/vscode-server-linux-x64-web.tar.gz -O /tmp/vscode-server-linux-${LATEST}-x64-web.tar.gz
sudo mkdir -p /opt/vscode/${LATEST}
sudo tar --strip-components=1 -xf /tmp/vscode-server-linux-${LATEST}-x64-web.tar.gz -C /opt/vscode/${LATEST}
CODEVER="$(cat /opt/vscode/${LATEST}/package.json | jq -r '.version')"
sudo ln -sfn ${LATEST} /opt/vscode/${CODEVER}
sudo ln -sfn ${CODEVER} /opt/vscode/latest

if [ "${VSCODE_SERVER_DISTRO}" == 'insider' ]; then
	pushd /opt/vscode/latest/bin
	ln -s code-server-insiders code-server
	popd
fi

cat <<'EOF' > /usr/local/bin/vscode.sh
#!/bin/bash

VSCODE_SERVER_TOKEN=${VSCODE_SERVER_TOKEN}
VSCODE_SERVER_LOG=${VSCODE_SERVER_LOG:=info}
VSCODE_SERVER_BIND_ADDR=${VSCODE_SERVER_BIND_ADDR:=0.0.0.0}
VSCODE_SERVER_BIND_PORT=${VSCODE_SERVER_BIND_PORT:=8000}
VSCODE_SERVER_DATA_DIR="${VSCODE_SERVER_DATA_DIR:=${HOME}/.vscode-remote}"
PATH=/opt/vscode/latest/bin:$PATH

set -o pipefail -o nounset

: "${VSCODE_KEYRING_PASS:?Variable not set or empty}"

mkdir -p "${VSCODE_SERVER_DATA_DIR}/data/Machine"

ARGS="--force-disable-user-env \
	--accept-server-license-terms \
	--server-data-dir "${VSCODE_SERVER_DATA_DIR}" \
	--host ${VSCODE_SERVER_BIND_ADDR} \
	--port ${VSCODE_SERVER_BIND_PORT} \
	--log ${VSCODE_SERVER_LOG} \
	--extensions-download-dir "${VSCODE_SERVER_DATA_DIR}/extensionsCache" \
	--install-extension amazonwebservices.aws-toolkit-vscode \
	--install-extension GitHub.codespaces \
	--install-extension GitHub.github-vscode-theme \
	--install-extension github.vscode-github-actions \
	--install-extension GitHub.vscode-pull-request-github \
	--install-extension golang.go \
	--install-extension ms-azuretools.vscode-docker \
	--install-extension MS-CEINTL.vscode-language-pack-fr \
	--install-extension ms-kubernetes-tools.kind-vscode \
	--install-extension ms-kubernetes-tools.vscode-kubernetes-tools \
	--install-extension ms-python.python \
	--install-extension ms-python.vscode-pylance \
	--install-extension ms-vscode.cmake-tools \
	--install-extension ms-vscode.cpptools \
	--install-extension ms-vscode.cpptools-extension-pack \
	--install-extension ms-vscode.cpptools-themes \
	--install-extension PKief.material-icon-theme \
	--install-extension rebornix.ruby \
	--install-extension redhat.java \
	--install-extension redhat.vscode-xml \
	--install-extension redhat.vscode-yaml \
	--install-extension twxs.cmake \
	--install-extension vscjava.vscode-maven \
	--install-extension wingrunr21.vscode-ruby \
	--install-extension xdebug.php-debug \
	--install-extension xdebug.php-pack \
	--install-extension zobo.php-intellisense \
	--do-not-sync \
	--start-server \
	--enable-remote-auto-shutdown"

if [ -n "${VSCODE_SERVER_TOKEN}" ]; then
	echo -n "${VSCODE_SERVER_TOKEN}" > "${VSCODE_SERVER_DATA_DIR}/data/Machine/.connection-token"
	ARGS="${ARGS} --connection-token-file ${VSCODE_SERVER_DATA_DIR}/data/Machine/.connection-token"
else
	ARGS="${ARGS} --without-connection-token"
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
