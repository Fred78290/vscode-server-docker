#!/bin/bash

CURDIR=$(dirname $0)

pushd ${CURDIR}

CURDIR=$PWD
LISTEN_PORT=1443
DOMAIN_NAME=${VSCODE_HOSTNAME#*.}
VSCODE_SERVER_NAME=localhost.${DOMAIN_NAME}
VSCODE_OAUTH2_PROXY_PROVIDER=${VSCODE_OAUTH2_PROXY_PROVIDER:=${GOOGLE_OAUTH2_PROXY_PROVIDER}}
VSCODE_OAUTH2_PROXY_CLIENT_ID=${VSCODE_OAUTH2_PROXY_CLIENT_ID:=${GOOGLE_OAUTH2_PROXY_CLIENT_ID}}
VSCODE_OAUTH2_PROXY_CLIENT_SECRET=${VSCODE_OAUTH2_PROXY_CLIENT_SECRET:=${GOOGLE_OAUTH2_PROXY_CLIENT_SECRET}}
VSCODE_OAUTH2_PROXY_SCOPE=${VSCODE_OAUTH2_PROXY_SCOPE:=${GOOGLE_OAUTH2_PROXY_SCOPE}}	
VSCODE_OAUTH2_PROXY_COOKIE_SECRET=$(dd if=/dev/urandom bs=32 count=1 2>/dev/null | base64 | tr -d -- '\n' | tr -- '+/' '-_'; echo)

export OSDISTRO=$(uname -s)

if [ "${OSDISTRO}" = "Linux" ]; then
	NET_IF=$(ip route get 1 | awk '{print $5;exit}')
	EXTERNAL_IP=$(ip addr show ${NET_IF} | grep -m 1 "inet\s" | tr '/' ' ' | awk '{print $2}')
else
	NET_IF=$(route get 1 | grep -m 1 interface | awk '{print $2}')
	EXTERNAL_IP=$(ifconfig ${NET_IF} | grep -m 1 "inet\s" | sed -n 1p | awk '{print $2}')
fi

if [ ! -f ${CURDIR}/ssl/privkey.pem ]; then
	openssl req -nodes -x509 -sha256 -newkey rsa:4096 \
	-keyout ${CURDIR}/ssl/privkey.pem \
	-out ${CURDIR}/ssl/cert.pem \
	-days 3560 \
	-subj "/C=US/ST=California/L=San Francisco/O=GitHub/OU=Fred78290/CN=localhost" \
	-extensions san \
	-config <( \
	echo '[req]'; \
	echo 'distinguished_name=req'; \
	echo '[san]'; \
	echo "subjectAltName=DNS:localhost,DNS:${VSCODE_SERVER_NAME}")
fi

docker network create vscode &> /dev/null || true

docker rm -f oauth2-proxy
docker run -d --restart always \
	--name oauth2-proxy \
	--net vscode \
	--hostname oauth2-proxy.${DOMAIN_NAME} \
	-e OAUTH2_PROXY_SESSION_STORE_TYPE=cookie \
	-e OAUTH2_PROXY_COOKIE_SAMESITE=lax \
	-e OAUTH2_PROXY_REVERSE_PROXY=true \
	-e OAUTH2_PROXY_COOKIE_CSRF_PER_REQUEST=true \
	-e OAUTH2_PROXY_COOKIE_CSRF_EXPIRE=60m \
	-e OAUTH2_PROXY_SKIP_PROVIDER_BUTTON=false \
	-e OAUTH2_PROXY_PASS_USER_HEADERS=true \
	-e OAUTH2_PROXY_SET_XAUTHREQUEST=true \
	-p 4180:4180 \
	quay.io/oauth2-proxy/oauth2-proxy:latest \
		--provider=${VSCODE_OAUTH2_PROXY_PROVIDER} \
		--client-id=${VSCODE_OAUTH2_PROXY_CLIENT_ID} \
		--client-secret=${VSCODE_OAUTH2_PROXY_CLIENT_SECRET} \
		--scope="${VSCODE_OAUTH2_PROXY_SCOPE}" \
		--cookie-secret=9t7ZhuK0qj1-FiEQr5K4Q8hJSLYfuK7_2NAVDIhXmVc= \
		--redirect-url=https://${VSCODE_SERVER_NAME}:1443/oauth2/callback \
		--cookie-domain=${DOMAIN_NAME} \
		--whitelist-domain=*.${DOMAIN_NAME} \
		--email-domain=* \
		--upstream=file:///dev/null \
		--http-address=0.0.0.0:4180


docker rm -f vscode-server-helper-wrapper
docker run -d --restart always \
	--name vscode-server-helper-wrapper \
	--hostname vscode-server-helper-wrapper.${DOMAIN_NAME} \
	--net vscode \
	-e EXTERNAL_IP=${EXTERNAL_IP} \
	-e VSCODE_SERVER_NAME=${VSCODE_SERVER_NAME} \
	-e OAUTH2_PROXY=oauth2-proxy.${DOMAIN_NAME} \
	-e LISTEN_PORT=${LISTEN_PORT} \
	-p ${LISTEN_PORT}:${LISTEN_PORT} \
	-v ${PWD}/ssl:/etc/nginx/ssl \
	-v ${PWD}/etc/template.d:/etc/nginx/templates/ \
	nginx
