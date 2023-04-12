#!/bin/bash
set -o pipefail -o nounset

export VSCODE_SERVER_IMAGE=${VSCODE_SERVER_IMAGE:=fred78290/vscode-server:v0.1.0}
export VSCODE_AUTH_TYPE=oauth2
export VSCODE_USER=$USER
export VSCODE_PASSWORD=$(uuidgen)
export VSCODE_NAMESPACE=${VSCODE_NAMESPACE:=vscode-server}
export VSCODE_HOSTNAME=${VSCODE_HOSTNAME:=${VSCODE_NAMESPACE}.acme.com}
export VSCODE_KEYRING_PASS=$(uuidgen)
export VSCODE_OAUTH2_PROXY_PROVIDER=${VSCODE_OAUTH2_PROXY_PROVIDER:=google}
export VSCODE_OAUTH2_PROXY_CLIENT_ID=${VSCODE_OAUTH2_PROXY_CLIENT_ID:-}
export VSCODE_OAUTH2_PROXY_CLIENT_SECRET=${VSCODE_OAUTH2_PROXY_CLIENT_SECRET:-}
export VSCODE_OAUTH2_PROXY_COOKIE_SECRET=
export VSCODE_CPU_REQUEST=500m
export VSCODE_MEM_REQUEST=512Mi
export VSCODE_CPU_MAX=4
export VSCODE_MEM_MAX=8G
export DIND_CPU_REQUEST=200m
export DIND_MEM_REQUEST=512Mi
export DIND_CPU_MAX=4
export DIND_MEM_MAX=4G
export VSCODE_ENVSUBST_FILTER="${VSCODE_ENVSUBST_FILTER:-}"

export NGINX_INGRESS_CLASS=$(kubectl get ingressclass -o json | jq -r '.items[]|select(.metadata.annotations."ingressclass.kubernetes.io/is-default-class" == "true")|.metadata.name')
export DRY_RUN=${DRY_RUN:-}

echo "On prompt hit return for default value"

read    -p "Enter your vscode hostname (${VSCODE_HOSTNAME}): " WANTED_VSCODE_HOSTNAME
read    -p "Enter your vscode auth type (oauth2|basic|none) (${VSCODE_AUTH_TYPE}): " WANTED_VSCODE_AUTH_TYPE

VSCODE_AUTH_TYPE=${WANTED_VSCODE_AUTH_TYPE:=${VSCODE_AUTH_TYPE}}
VSCODE_HOSTNAME=${WANTED_VSCODE_HOSTNAME:=${VSCODE_HOSTNAME}}

if [ "${VSCODE_AUTH_TYPE}" == basic ]; then
	read    -p "Enter your vscode username (${VSCODE_USER}): " WANTED_VSCODE_USER
	read -s -p "Enter your vscode password (${VSCODE_PASSWORD}): " WANTED_VSCODE_PASSWORD ; echo

	VSCODE_USER=${WANTED_VSCODE_USER:=${VSCODE_USER}}
	VSCODE_PASSWORD=${WANTED_VSCODE_PASSWORD:=${VSCODE_PASSWORD}}

	echo "${VSCODE_PASSWORD}" | htpasswd -i -c auth ${VSCODE_USER}

	DEFINED_ENVS=$(printf '${%s} ' $(awk "END { for (name in ENVIRON) { print ( name ~ /${VSCODE_ENVSUBST_FILTER}/ ) ? name : \"\" } }" < /dev/null ))

cat <<EOF | envsubst "$DEFINED_ENVS" | tee kubernetes/deployed.yml | kubectl apply ${DRY_RUN} -f -
$(cat kubernetes/basic.yaml)
---
$(kubectl create secret generic basic-auth -n ${VSCODE_NAMESPACE} --from-file=auth --from-literal=VSCODE_USER=${VSCODE_USER} --from-literal=VSCODE_PASSWORD=${VSCODE_PASSWORD} --dry-run=client -o yaml)
EOF

elif [ "${VSCODE_AUTH_TYPE}" == oauth2 ]; then
	VSCODE_OAUTH2_PROXY_COOKIE_SECRET=$(dd if=/dev/urandom bs=32 count=1 2>/dev/null | base64 | tr -d -- '\n' | tr -- '+/' '-_'; echo)

	while [ -z "${VSCODE_OAUTH2_PROXY_CLIENT_ID}" ] || [ -z "${VSCODE_OAUTH2_PROXY_CLIENT_SECRET}" ] || [ -z "${VSCODE_OAUTH2_PROXY_PROVIDER}" ];
	do
		read -p "Enter your oauth2 provider (${VSCODE_OAUTH2_PROXY_PROVIDER}): " WANTED_VSCODE_OAUTH2_PROXY_PROVIDER
		read -p "Enter your oauth2 client id (${VSCODE_OAUTH2_PROXY_CLIENT_ID}): " WANTED_OAUTH2_PROXY_CLIENT_ID
		read -p "Enter your oauth2 client secret (${VSCODE_OAUTH2_PROXY_CLIENT_SECRET}): " WANTED_OAUTH2_PROXY_CLIENT_SECRET

		VSCODE_OAUTH2_PROXY_PROVIDER=${WANTED_VSCODE_OAUTH2_PROXY_PROVIDER:=${VSCODE_OAUTH2_PROXY_PROVIDER}}
		VSCODE_OAUTH2_PROXY_CLIENT_ID=${WANTED_OAUTH2_PROXY_CLIENT_ID:=${VSCODE_OAUTH2_PROXY_CLIENT_ID}}
		VSCODE_OAUTH2_PROXY_CLIENT_SECRET=${WANTED_OAUTH2_PROXY_CLIENT_SECRET:=${VSCODE_OAUTH2_PROXY_CLIENT_SECRET}}
	done

	DEFINED_ENVS=$(printf '${%s} ' $(awk "END { for (name in ENVIRON) { print ( name ~ /${VSCODE_ENVSUBST_FILTER}/ ) ? name : \"\" } }" < /dev/null ))

	cat kubernetes/oauth.yaml | envsubst "$DEFINED_ENVS" | tee kubernetes/deployed.yml | kubectl apply ${DRY_RUN} -f -
elif [ "${VSCODE_AUTH_TYPE}" == none ]; then
	DEFINED_ENVS=$(printf '${%s} ' $(awk "END { for (name in ENVIRON) { print ( name ~ /${VSCODE_ENVSUBST_FILTER}/ ) ? name : \"\" } }" < /dev/null ))

	cat kubernetes/none.yaml | envsubst "$DEFINED_ENVS" | tee kubernetes/deployed.yml | kubectl apply ${DRY_RUN} -f -
else
	echo "Authentification mode: ${VSCODE_AUTH_TYPE}, not supported"
	exit 1
fi
