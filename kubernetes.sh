#!/bin/bash
set -o pipefail -o nounset

: "${VSCODE_SERVER_REGISTRY:?Variable not set or empty}"

export VSCODE_ACCOUNT_TYPE=single
export VSCODE_SERVER_REGISTRY=${DEV_VSCODE_SERVER_REGISTRY:=${VSCODE_SERVER_REGISTRY}}
export VSCODE_SERVER_IMAGE=${VSCODE_SERVER_IMAGE:=${VSCODE_SERVER_REGISTRY}/vscode-server:v0.1.0}
export VSCODE_SERVER_HELPER_IMAGE=${VSCODE_SERVER_HELPER_IMAGE:=${VSCODE_SERVER_REGISTRY}/vscode-server-helper:v0.1.0}
export VSCODE_AUTH_TYPE=oauth2
export VSCODE_USER=$USER
export VSCODE_PASSWORD=$(uuidgen)
export VSCODE_NAMESPACE=${VSCODE_NAMESPACE:=vscode-server}
export VSCODE_HOSTNAME=${DEV_VSCODE_HOSTNAME:=${VSCODE_HOSTNAME:=${VSCODE_NAMESPACE}.acme.com}}
export VSCODE_KEYRING_PASS=$(uuidgen)
export VSCODE_PVC_SIZE=10Gi

export VSCODE_OAUTH2_PROXY_PROVIDER=${VSCODE_OAUTH2_PROXY_PROVIDER:=${GITHUB_OAUTH2_PROXY_PROVIDER}}
export VSCODE_OAUTH2_PROXY_CLIENT_ID=${VSCODE_OAUTH2_PROXY_CLIENT_ID:=${GITHUB_OAUTH2_PROXY_CLIENT_ID}}
export VSCODE_OAUTH2_PROXY_CLIENT_SECRET=${VSCODE_OAUTH2_PROXY_CLIENT_SECRET:=${GITHUB_OAUTH2_PROXY_CLIENT_SECRET}}
export VSCODE_OAUTH2_PROXY_SCOPE=${VSCODE_OAUTH2_PROXY_SCOPE:=${GITHUB_OAUTH2_PROXY_SCOPE}}	
export VSCODE_OAUTH2_PROXY_COOKIE_SECRET=$(dd if=/dev/urandom bs=32 count=1 2>/dev/null | base64 | tr -d -- '\n' | tr -- '+/' '-_'; echo)

export VSCODE_CPU_REQUEST=500m
export VSCODE_MEM_REQUEST=512Mi
export VSCODE_CPU_MAX=4
export VSCODE_MEM_MAX=8G
export DIND_CPU_REQUEST=200m
export DIND_MEM_REQUEST=512Mi
export DIND_CPU_MAX=4
export DIND_MEM_MAX=4G
export VSCODE_ENVSUBST_FILTER="${VSCODE_ENVSUBST_FILTER:-}"
export VSCODE_INGRESS_AUTH_URL='https://$host/oauth2/auth'
export VSCODE_INGRESS_AUTH_SIGNIN='https://$host/oauth2/start?rd=$escaped_request_uri'
export VSCODE_CERT_CLUSTER_ISSUER='letsencrypt-prod'

export NGINX_INGRESS_CLASS=$(kubectl get ingressclass -o json | jq -r '.items[]|select(.metadata.annotations."ingressclass.kubernetes.io/is-default-class" == "true")|.metadata.name')
export DRY_RUN=${DRY_RUN:-}

function url_encode() {
    echo "$@" \
    | sed \
        -e 's/%/%25/g' \
        -e 's/ /%20/g' \
        -e 's/!/%21/g' \
        -e 's/"/%22/g' \
        -e "s/'/%27/g" \
        -e 's/#/%23/g' \
        -e 's/(/%28/g' \
        -e 's/)/%29/g' \
        -e 's/+/%2b/g' \
        -e 's/,/%2c/g' \
        -e 's/-/%2d/g' \
        -e 's/:/%3a/g' \
        -e 's/;/%3b/g' \
        -e 's/?/%3f/g' \
        -e 's/@/%40/g' \
        -e 's/\$/%24/g' \
        -e 's/\&/%26/g' \
        -e 's/\*/%2a/g' \
        -e 's/\./%2e/g' \
        -e 's/\//%2f/g' \
        -e 's/\[/%5b/g' \
        -e 's/\\/%5c/g' \
        -e 's/\]/%5d/g' \
        -e 's/\^/%5e/g' \
        -e 's/_/%5f/g' \
        -e 's/`/%60/g' \
        -e 's/{/%7b/g' \
        -e 's/|/%7c/g' \
        -e 's/}/%7d/g' \
        -e 's/~/%7e/g'
}

echo "On prompt hit return for default value"

read    -p "Enter account type (single|multi) (${VSCODE_ACCOUNT_TYPE}): " WANTED_VSCODE_ACCOUNT_TYPE
read    -p "Enter your vscode hostname (${VSCODE_HOSTNAME}): " WANTED_VSCODE_HOSTNAME
read    -p "Enter your vscode auth type (oauth2|basic|none) (${VSCODE_AUTH_TYPE}): " WANTED_VSCODE_AUTH_TYPE

VSCODE_ACCOUNT_TYPE=${WANTED_VSCODE_AUTH_TYPE:=${WANTED_VSCODE_ACCOUNT_TYPE}}
VSCODE_AUTH_TYPE=${WANTED_VSCODE_AUTH_TYPE:=${VSCODE_AUTH_TYPE}}
VSCODE_HOSTNAME=${WANTED_VSCODE_HOSTNAME:=${VSCODE_HOSTNAME}}

if [ ${VSCODE_ACCOUNT_TYPE} == "single" ]; then

	if [ "${VSCODE_AUTH_TYPE}" == basic ]; then
		read    -p "Enter your vscode username (${VSCODE_USER}): " WANTED_VSCODE_USER
		read -s -p "Enter your vscode password (${VSCODE_PASSWORD}): " WANTED_VSCODE_PASSWORD ; echo

		VSCODE_USER=${WANTED_VSCODE_USER:=${VSCODE_USER}}
		VSCODE_PASSWORD=${WANTED_VSCODE_PASSWORD:=${VSCODE_PASSWORD}}

		echo "${VSCODE_PASSWORD}" | htpasswd -i -c auth ${VSCODE_USER}

		DEFINED_ENVS=$(printf '${%s} ' $(awk "END { for (name in ENVIRON) { print ( name ~ /${VSCODE_ENVSUBST_FILTER}/ ) ? name : \"\" } }" < /dev/null ))

	cat <<EOF | envsubst "$DEFINED_ENVS" | tee kubernetes/single-account/deployed.yml | kubectl apply ${DRY_RUN} -f -
	$(cat kubernetes/single-account/basic.yaml)
	---
	$(kubectl create secret generic basic-auth -n ${VSCODE_NAMESPACE} --from-file=auth --from-literal=VSCODE_USER=${VSCODE_USER} --from-literal=VSCODE_PASSWORD=${VSCODE_PASSWORD} --dry-run=client -o yaml)
EOF

	elif [ "${VSCODE_AUTH_TYPE}" == oauth2 ]; then
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

		cat kubernetes/single-account/oauth.yaml | envsubst "$DEFINED_ENVS" | tee kubernetes/single-account/deployed.yml | kubectl apply ${DRY_RUN} -f -
	elif [ "${VSCODE_AUTH_TYPE}" == none ]; then
		DEFINED_ENVS=$(printf '${%s} ' $(awk "END { for (name in ENVIRON) { print ( name ~ /${VSCODE_ENVSUBST_FILTER}/ ) ? name : \"\" } }" < /dev/null ))

		cat kubernetes/single-account/none.yaml | envsubst "$DEFINED_ENVS" | tee kubernetes/single-account/deployed.yml | kubectl apply ${DRY_RUN} -f -
	else
		echo "Authentification mode: ${VSCODE_AUTH_TYPE}, not supported"
		exit 1
	fi
else
	VSCODE_SERVER_REDIRECT=$(url_encode "https://${VSCODE_HOSTNAME}/create-space")

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

	cat <<EOF | envsubst "$DEFINED_ENVS" | tee kubernetes/multi-account/deployed.yml | kubectl apply ${DRY_RUN} -f -
	$(cat kubernetes/multi-account/main.yaml)
	---
	$(kubectl create secret tls vscode-server-ingress-tls -n ${VSCODE_NAMESPACE} --key nginx/ssl/privkey.pem --cert nginx/ssl/cert.pem --dry-run=client -o yaml)
	---
	$(kubectl create configmap vscode-server-template -n ${VSCODE_NAMESPACE} --from-file=kubernetes/multi-account/template.yaml --dry-run=client -o yaml)
EOF

fi