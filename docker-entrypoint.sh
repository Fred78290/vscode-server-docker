#!/bin/bash

ARGS="$@"

if [ -z "$ARGS" ]; then
	ARGS=serve-local
fi

code-server --accept-server-license-terms update

/usr/bin/sudo /usr/sbin/nginx -c /etc/nginx/nginx.conf -g "daemon on; master_process on;"

exec code-server --accept-server-license-terms ${ARGS}