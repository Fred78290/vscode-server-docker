#!/bin/bash
# vim:sw=4:ts=4:et

set -e
set -o pipefail -o nounset

: "${VSCODE_KEYRING_PASS:?Variable not set or empty}"

ARGS="$@"

echo $ARGS

if [ -z "$ARGS" ]; then
	ARGS="serve-local --accept-server-license-terms --without-connection-token --host 0.0.0.0"
fi

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

#exec dbus-run-session -- code-server --accept-server-license-terms ${ARGS} --host 0.0.0.0

exec dbus-run-session -- sh -c "(echo $VSCODE_KEYRING_PASS | gnome-keyring-daemon --unlock) && code-server ${ARGS}"
