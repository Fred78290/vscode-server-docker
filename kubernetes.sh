#!/bin/bash
export VSCODE_USER=$USER
export VSCODE_PASSWORD=$(uuidgen)
export VSCODE_HOSTNAME=vscode-server.acme.com
export VSCODE_KEYRING_PASS=$(uuidgen)

read    -p "Enter your vscode username (${VSCODE_USER}): " WANTED_VSCODE_USER
read -s -p "Enter your vscode password (${VSCODE_PASSWORD}): " WANTED_VSCODE_PASSWORD ; echo
read    -p "Enter your vscode hostname (${VSCODE_HOSTNAME}): " WANTED_VSCODE_HOSTNAME

if [ -n "${WANTED_VSCODE_USER}" ]; then
	VSCODE_USER=${WANTED_VSCODE_USER}
fi

if [ -n "${WANTED_VSCODE_PASSWORD}" ]; then
	VSCODE_PASSWORD=${WANTED_VSCODE_PASSWORD}
fi

if [ -n "${WANTED_VSCODE_HOSTNAME}" ]; then
	VSCODE_HOSTNAME=${WANTED_VSCODE_HOSTNAME}
fi

echo "${VSCODE_PASSWORD}" | htpasswd -i -c auth ${VSCODE_USER}

DEFINED_ENVS=$(printf '${%s} ' $(awk "END { for (name in ENVIRON) { print ( name ~ /${filter}/ ) ? name : \"\" } }" < /dev/null ))

cat <<EOF | envsubst "$DEFINED_ENVS" | tee deployed.yml | kubectl apply -f -
$(cat kubernetes.yaml)
---
$(kubectl create secret generic basic-auth -n vscode-server --from-file=auth --from-literal=VSCODE_USER=${VSCODE_USER} --from-literal=VSCODE_PASSWORD=${VSCODE_PASSWORD} --dry-run=client -o yaml)
EOF
