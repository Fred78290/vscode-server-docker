#!/bin/bash

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
LATEST="$(curl -fsSL https://update.code.visualstudio.com/api/commits/stable/server-linux-x64-web | jq -r 'first')"
wget https://az764295.vo.msecnd.net/stable/${LATEST}/vscode-server-linux-x64-web.tar.gz -O /tmp/vscode-server-linux-${LATEST}-x64-web.tar.gz
sudo mkdir -p /opt/vscode/${LATEST}
sudo tar --strip-components=1 -xf /tmp/vscode-server-linux-${LATEST}-x64-web.tar.gz -C /opt/vscode/${LATEST}
CODEVER="$(cat /opt/vscode/${LATEST}/package.json | jq -r '.version')"
sudo ln -sfn ${LATEST} /opt/vscode/${CODEVER}
sudo ln -sfn ${CODEVER} /opt/vscode/latest

cat <<'EOF' > /usr/local/bin/vscode.sh
#!/bin/bash

set -o pipefail -o nounset

: "${VSCODE_KEYRING_PASS:?Variable not set or empty}"

export VSCODE_SERVER_DATA_DIR="${VSCODE_SERVER_DATA_DIR:=$HOME/.vscode-remote}"
export PATH=/opt/vscode/latest/bin:$PATH

ARGS="$@"

if [ -z "$ARGS" ]; then
	ARGS="serve-local --accept-server-license-terms --without-connection-token --host 0.0.0.0 --log debug"
fi

exec dbus-run-session -- sh -c "(echo $VSCODE_KEYRING_PASS | gnome-keyring-daemon --unlock) && code-server ${ARGS}"

EOF

chmod +x /usr/local/bin/vscode.sh

apt autoclean
apt-get clean -y
rm -rf  /var/lib/apt/lists/*
