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
apt install nano sudo python3 jq gettext-base wget curl git build-essential openssh-client iproute2 libsecret-1-0 dbus-user-session gnome-keyring ca-certificates zlib1g -y --no-install-recommends

curl -sL https://aka.ms/install-vscode-server/setup.sh | sh

cat <<'EOF' > /usr/local/bin/vscode.sh
#!/bin/bash

set -o pipefail -o nounset

: "${VSCODE_KEYRING_PASS:?Variable not set or empty}"

export VSCODE_SERVER_DATA_DIR="${VSCODE_SERVER_DATA_DIR:=$HOME/.vscode-remote}"

ARGS="$@"

echo $ARGS

if [ -z "$ARGS" ]; then
	ARGS="serve-local --accept-server-license-terms --without-connection-token --host 0.0.0.0"
fi

code-server --accept-server-license-terms update

exec dbus-run-session -- sh -c "(echo $VSCODE_KEYRING_PASS | gnome-keyring-daemon --unlock) && code-server ${ARGS}"

EOF

chmod +x /usr/local/bin/vscode.sh

apt autoclean
apt-get clean -y
rm -rf  /var/lib/apt/lists/*
