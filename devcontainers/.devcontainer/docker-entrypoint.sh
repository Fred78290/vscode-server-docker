#!/bin/bash
# vim:sw=4:ts=4:et

ARGS=$0

set -e
set -o pipefail -o nounset

: "${VSCODE_KEYRING_PASS:?Variable not set or empty}"

source /lib/lsb/init-functions

entrypoint_log() {
    if [ -z "${NGINX_ENTRYPOINT_QUIET_LOGS:-}" ]; then
        echo "$@"
    fi
}

if /usr/bin/find "/docker-entrypoint.d/" -mindepth 1 -maxdepth 1 -type f -print -quit 2>/dev/null | read v; then
	entrypoint_log "$0: /docker-entrypoint.d/ is not empty, will attempt to perform configuration"

	entrypoint_log "$0: Looking for shell scripts in /docker-entrypoint.d/"
	find "/docker-entrypoint.d/" -follow -type f -print | sort -V | while read -r f; do
		case "$f" in
			*.envsh)
				if [ -x "$f" ]; then
					entrypoint_log "$0: Sourcing $f";
					. "$f"
				else
					# warn on shell scripts without exec bit
					entrypoint_log "$0: Ignoring $f, not executable";
				fi
				;;
			*.sh)
				if [ -x "$f" ]; then
					entrypoint_log "$0: Launching $f";
					"$f"
				else
					# warn on shell scripts without exec bit
					entrypoint_log "$0: Ignoring $f, not executable";
				fi
				;;
			*) entrypoint_log "$0: Ignoring $f";;
		esac
	done

	entrypoint_log "$0: Configuration complete; ready for start up"
else
	entrypoint_log "$0: No files found in /docker-entrypoint.d/, skipping configuration"
fi

count=0

while [[ ! -f /var/run/docker-host.sock && $count -lt 5 ]];
do
	echo "not found"
	count=$((count+1))
done

start-stop-daemon --start --quiet --oknodo --chuid ${USER_ID}:${GROUP_ID} --pidfile /tmp/vscode.pid --exec /usr/local/bin/vscode.sh -- $ARGS

set +e
exec "$@"
