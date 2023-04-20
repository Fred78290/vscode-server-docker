#!/bin/bash
set -o pipefail -o nounset

if [ "$(uname -s)" == "Darwin" ]; then
    if [ -z "$(command -v gsed)" ]; then
        echo_red_bold "You must install gnu sed with brew (brew install gsed), this script is not compatible with the native macos sed"
        exit 1
    fi

    if [ -z "$(command -v gbase64)" ]; then
        echo_red_bold "You must install gnu base64 with brew (brew install coreutils), this script is not compatible with the native macos base64"
        exit 1
    fi

    if [ ! -e /usr/local/opt/gnu-getopt/bin/getopt ]; then
        echo_red_bold "You must install gnu gnu-getopt with brew (brew install coreutils), this script is not compatible with the native macos base64"
        exit 1
    fi

    shopt -s expand_aliases

    alias base64=gbase64
    alias sed=gsed
    alias getopt=/usr/local/opt/gnu-getopt/bin/getopt

	export NET_IF=$(route get 1 | grep -m 1 interface | awk '{print $2}')
	export EXTERNAL_IP=$(ifconfig ${NET_IF} | grep -m 1 "inet\s" | sed -n 1p | awk '{print $2}')
else
	export NET_IF=$(ip route get 1 | awk '{print $5;exit}')
	export EXTERNAL_IP=$(ip addr show ${NET_IF} | grep -m 1 "inet\s" | tr '/' ' ' | awk '{print $2}')
fi

: "${VSCODE_SERVER_REGISTRY:?Variable not set or empty}"

export VSCODE_ACCOUNT_TYPE=multi
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

export VSCODE_OAUTH2_PROXY_PROVIDER=${VSCODE_OAUTH2_PROXY_PROVIDER:=${GITHUB_DEBUG_OAUTH2_PROXY_PROVIDER}}
export VSCODE_OAUTH2_PROXY_CLIENT_ID=${VSCODE_OAUTH2_PROXY_CLIENT_ID:=${GITHUB_DEBUG_OAUTH2_PROXY_CLIENT_ID}}
export VSCODE_OAUTH2_PROXY_CLIENT_SECRET=${VSCODE_OAUTH2_PROXY_CLIENT_SECRET:=${GITHUB_DEBUG_OAUTH2_PROXY_CLIENT_SECRET}}
export VSCODE_OAUTH2_PROXY_SCOPE=${VSCODE_OAUTH2_PROXY_SCOPE:=${GITHUB_DEBUG_OAUTH2_PROXY_SCOPE}}	
export VSCODE_OAUTH2_PROXY_COOKIE_SECRET=$(dd if=/dev/urandom bs=32 count=1 2>/dev/null | base64 | tr -d -- '\n' | tr -- '+/' '-_'; echo)

export VSCODE_OAUTH2_PROXY_PROVIDER=${GOOGLE_OAUTH2_PROXY_PROVIDER}
export VSCODE_OAUTH2_PROXY_CLIENT_ID=${GOOGLE_OAUTH2_PROXY_CLIENT_ID}
export VSCODE_OAUTH2_PROXY_CLIENT_SECRET=${GOOGLE_OAUTH2_PROXY_CLIENT_SECRET}
export VSCODE_OAUTH2_PROXY_SCOPE=${GOOGLE_OAUTH2_PROXY_SCOPE}

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
export VSCODE_MODE=dev
export VSCODE_TLS_KEY=nginx/ssl/privkey.pem
export VSCODE_TLS_CERT=nginx/ssl/cert.pem

export NGINX_INGRESS_CLASS=$(kubectl get ingressclass -o json | jq -r '.items[]|select(.metadata.annotations."ingressclass.kubernetes.io/is-default-class" == "true")|.metadata.name')
export DRY_RUN=${DRY_RUN:-}

NGINX_INGRESS_CLASS=${NGINX_INGRESS_CLASS:=nginx}

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

function usage {
	echo "TODO"
	exit 0
} 

TEMP=$(getopt -o hvxr --long mode:,account-type:,auth-type:,user:,password:,namespace:,hostname:,oauth2-proxy-provider:,oauth2-proxy-client-id:,oauth2-proxy-client-secret:,oauth2-proxy-scope:,vscode-server-cpu-request:,vscode-server-cpu-max:,vscode-server-mem-request:,vscode-server-mem-max:,vscode-server-volume-size:,vscode-server-image:,vscode-server-helper-image:,vscode-server-registry: -n "$0" -- "$@")

eval set -- "${TEMP}"

while true; do
    case "$1" in
    -h|--help)
        usage
        exit 0
        ;;
	--mode)
		VSCODE_MODE=$2
		shift 2
		;;
	--account-type)
		VSCODE_ACCOUNT_TYPE=$2
		shift 2
		;;
	--auth-type)
		VSCODE_AUTH_TYPE=$2
		shift 2
		;;
	--user)
		VSCODE_USER=$2
		shift 2
		;;
	--password)
		VSCODE_PASSWORD=$2
		shift 2
		;;
	--namespace)
		VSCODE_NAMESPACE=$2
		shift 2
		;;
	--hostname)
		VSCODE_HOSTNAME=$2
		shift 2
		;;

	--oauth2-proxy-provider)
		VSCODE_OAUTH2_PROXY_PROVIDER=$2
		shift 2
		;;
	--oauth2-proxy-client-id)
		VSCODE_OAUTH2_PROXY_CLIENT_ID=$2
		shift 2
		;;
	--oauth2-proxy-client-secret)
		VSCODE_OAUTH2_PROXY_CLIENT_SECRET=$2
		shift 2
		;;
	--oauth2-proxy-scope)
		VSCODE_OAUTH2_PROXY_SCOPE=$2
		shift 2
		;;

	--oauth-url)
		VSCODE_INGRESS_AUTH_URL=$2
		shift 2
		;;
	--oauth-signin)
		VSCODE_INGRESS_AUTH_SIGNIN=$2
		shift 2
		;;
	
	--vscode-server-cpu-request)
		VSCODE_CPU_REQUEST=$2
		shift 2
		;;
	--vscode-server-cpu-max)
		VSCODE_CPU_MAX=$2
		shift 2
		;;
	--vscode-server-mem-request)
		VSCODE_MEM_REQUEST=$2
		shift 2
		;;
	--vscode-server-mem-max)
		VSCODE_MEM_MAX=$2
		shift 2
		;;
	--vscode-server-volume-size)
		VSCODE_PVC_SIZE=$2
		shift 2
		;;

	--vscode-server-image)
		VSCODE_SERVER_IMAGE=$2
		shift 2
		;;
	--vscode-server-helper-image)
		VSCODE_SERVER_HELPER_IMAGE=$2
		shift 2
		;;
	--vscode-server-registry)
		VSCODE_SERVER_REGISTRY=$2
		shift 2
		;;

	--tls-key)
		VSCODE_TLS_KEY=$2
		shift 2
		;;
	--tls-cert)
		VSCODE_TLS_CERT=$2
		shift 2
		;;

	--dry-run)
		DRY_RUN=--dry-run=client
		shift 1
		;;
    --)
        shift
        break
        ;;
    *)
        echo_red "$1 - Internal error!"
        exit 1
        ;;
    esac
done

VSCODE_SERVER_REDIRECT=$(url_encode "https://${VSCODE_HOSTNAME}/create-space")

: "${VSCODE_MODE:?Variable not set or empty}"
: "${VSCODE_HOSTNAME:?Variable not set or empty}"
: "${VSCODE_NAMESPACE:?Variable not set or empty}"
: "${VSCODE_ACCOUNT_TYPE:?Variable not set or empty}"
: "${VSCODE_OAUTH2_PROXY_PROVIDER:?Variable not set or empty}"
: "${VSCODE_OAUTH2_PROXY_CLIENT_ID:?Variable not set or empty}"
: "${VSCODE_OAUTH2_PROXY_CLIENT_SECRET:?Variable not set or empty}"
: "${VSCODE_OAUTH2_PROXY_SCOPE:?Variable not set or empty}"

: "${VSCODE_INGRESS_AUTH_URL:?Variable not set or empty}"
: "${VSCODE_INGRESS_AUTH_SIGNIN:?Variable not set or empty}"

: "${VSCODE_CPU_REQUEST:?Variable not set or empty}"
: "${VSCODE_CPU_MAX:?Variable not set or empty}"
: "${VSCODE_MEM_REQUEST:?Variable not set or empty}"
: "${VSCODE_MEM_MAX:?Variable not set or empty}"
: "${VSCODE_PVC_SIZE:?Variable not set or empty}"

: "${VSCODE_SERVER_IMAGE:?Variable not set or empty}"
: "${VSCODE_SERVER_HELPER_IMAGE:?Variable not set or empty}"

if [ -z "${VSCODE_TLS_KEY}" ] && [ -z "${VSCODE_TLS_CERT}" ]; then
	VSCODE_INGRESS_AUTH_URL=$(echo -n ${VSCODE_INGRESS_AUTH_URL} | sed -e 's/https/http/')
	VSCODE_INGRESS_AUTH_SIGNIN=$(echo -n {VSCODE_INGRESS_AUTH_SIGNIN}  | sed -e 's/https/http/')
fi

DEFINED_ENVS=$(printf '${%s} ' $(awk "END { for (name in ENVIRON) { print ( name ~ /${VSCODE_ENVSUBST_FILTER}/ ) ? name : \"\" } }" < /dev/null ))

touch auth

if [ ${VSCODE_ACCOUNT_TYPE} == "single" ]; then
	if [ "${VSCODE_AUTH_TYPE}" == basic ]; then
		: "${VSCODE_USER:?Variable not set or empty}"
		: "${VSCODE_PASSWORD:?Variable not set or empty}"
		
		echo "${VSCODE_PASSWORD}" | htpasswd -i -c auth ${VSCODE_USER}
	
		MAIN_TEMPLATE=kubernetes/single-account/basic.yaml
	elif [ "${VSCODE_AUTH_TYPE}" == oauth2 ]; then
		MAIN_TEMPLATE=kubernetes/single-account/oauth.yaml
	elif [ "${VSCODE_AUTH_TYPE}" == none ]; then
		MAIN_TEMPLATE=kubernetes/single-account/none.yaml
	else
		echo "Authentification mode: ${VSCODE_AUTH_TYPE}, not supported"
		exit 1
	fi

	if [ -n "${VSCODE_TLS_KEY}" ] && [ -n "${VSCODE_TLS_CERT}" ]; then
		cat <<EOF | envsubst "$DEFINED_ENVS" | tee kubernetes/multi-account/deployed.yml | kubectl apply ${DRY_RUN} -f -
$(cat ${MAIN_TEMPLATE})
---
$(kubectl create secret tls vscode-server-ingress-tls -n ${VSCODE_NAMESPACE} --key ${VSCODE_TLS_KEY} --cert ${VSCODE_TLS_CERT} --dry-run=client -o yaml)
---
$(kubectl create secret generic basic-auth -n ${VSCODE_NAMESPACE} --from-file=auth --from-literal=VSCODE_USER=${VSCODE_USER} --from-literal=VSCODE_PASSWORD=${VSCODE_PASSWORD} --dry-run=client -o yaml)
EOF
	else
		cat <<EOF | envsubst "$DEFINED_ENVS" | tee kubernetes/multi-account/deployed.yml | kubectl apply ${DRY_RUN} -f -
$(cat ${MAIN_TEMPLATE})
---
$(kubectl create secret generic basic-auth -n ${VSCODE_NAMESPACE} --from-file=auth --from-literal=VSCODE_USER=${VSCODE_USER} --from-literal=VSCODE_PASSWORD=${VSCODE_PASSWORD} --dry-run=client -o yaml)
EOF
	fi
else

	if [ ${VSCODE_MODE} == "dev" ]; then
		MAIN_TEMPLATE=kubernetes/multi-account/dev.yaml
	else
		MAIN_TEMPLATE=kubernetes/multi-account/main.yaml
	fi

	if [ -n "${VSCODE_TLS_KEY}" ] && [ -n "${VSCODE_TLS_CERT}" ]; then

		cat <<EOF | envsubst "$DEFINED_ENVS" | tee kubernetes/multi-account/deployed.yml | kubectl apply ${DRY_RUN} -f -
$(cat ${MAIN_TEMPLATE})
---
$(kubectl create secret tls vscode-server-ingress-tls -n ${VSCODE_NAMESPACE} --key ${VSCODE_TLS_KEY} --cert ${VSCODE_TLS_CERT} --dry-run=client -o yaml)
---
$(kubectl create configmap vscode-server-template -n ${VSCODE_NAMESPACE} --from-file=kubernetes/multi-account/template.yaml --dry-run=client -o yaml)
EOF

	else

		cat <<EOF | envsubst "$DEFINED_ENVS" | tee kubernetes/multi-account/deployed.yml | kubectl apply ${DRY_RUN} -f -
$(cat ${MAIN_TEMPLATE})
---
$(kubectl create configmap vscode-server-template -n ${VSCODE_NAMESPACE} --from-file=kubernetes/multi-account/template.yaml --dry-run=client -o yaml)
EOF

	fi

fi
