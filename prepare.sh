#!/bin/bash
set -e

export DEBIAN_FRONTEND=noninteractive

apt update
apt dist-upgrade -y
apt install nano sudo gettext-base wget curl git build-essential openssh-client iproute2 libsecret-1-0 dbus-user-session gnome-keyring ca-certificates zlib1g php nginx php-fpm php-intl php-xml php-soap php-redis php-curl php-imagick php-mbstring php-mysql php-sqlite3 -y --no-install-recommends

curl -fsSL https://cli.github.com/packages/githubcli-archive-keyring.gpg | dd of=/usr/share/keyrings/githubcli-archive-keyring.gpg 
chmod go+r /usr/share/keyrings/githubcli-archive-keyring.gpg
echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main" | tee /etc/apt/sources.list.d/github-cli.list > /dev/null
apt update
apt install gh  -y

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

mkdir -p /usr/local/yarn

curl -sL "https://download.docker.com/linux/static/stable/${DOCKER_ARCH}/docker-${DOCKER_VERSION}.tgz" | tar --strip-components 1 -C /usr/local/bin -xzf - 'docker/docker' 
curl -sL "https://dl.google.com/go/go${GOLANG_VERSION}.linux-${GO_ARCH}.tar.gz" | tar -C /usr/local -xzf -
curl -sL "https://nodejs.org/dist/v${NODE_VERSION}/node-v${NODE_VERSION}-linux-${ARCH}.tar.gz" | tar --strip-components=1 -C /usr/local -xzf -
curl -sL "https://yarnpkg.com/downloads/${YARN_VERSION}/yarn-v${YARN_VERSION}.tar.gz" | tar --strip-components=1 -C /usr/local/yarn -xzf -
curl -sL https://github.com/krallin/tini/releases/download/${TINI_VERSION}/tini-${GO_ARCH} -o /usr/local/bin/tini

ln -s /usr/local/bin/node /usr/local/bin/nodejs
ln -s /usr/local/yarn/bin/yarn /usr/local/bin/yarn
ln -s /usr/local/yarn/bin/yarnpkg /usr/local/bin/yarnpkg

curl -sL https://aka.ms/install-vscode-server/setup.sh | sh

# smoke tests
node --version
npm --version
yarn --version
go version
set -x
groupadd -g ${GROUP_ID} ${VSCODE_USER}
adduser --uid ${USER_ID} --gid ${GROUP_ID} --home /home/${VSCODE_USER} ${VSCODE_USER}
adduser ${VSCODE_USER} root
chown -R ${VSCODE_USER}:${VSCODE_USER} /home/${VSCODE_USER}
bash -c "echo '${VSCODE_USER} ALL=(ALL) NOPASSWD:ALL' > /etc/sudoers.d/${VSCODE_USER}"

chmod +x /usr/local/bin/tini /docker-entrypoint.sh

apt autoclean
apt-get clean -y
rm -rf  /usr/share/doc /usr/share/doc-base /var/lib/apt/lists/*

