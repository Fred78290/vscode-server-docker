#!/usr/bin/env bash
#-------------------------------------------------------------------------------------------------------------
# Copyright (c) Microsoft Corporation. All rights reserved.
# Licensed under the MIT License. See https://go.microsoft.com/fwlink/?linkid=2090316 for license information.
#-------------------------------------------------------------------------------------------------------------
#
# Docs: https://github.com/microsoft/vscode-dev-containers/blob/main/script-library/docs/docker-in-docker.md
# Maintainer: The Dev Container spec maintainers


DOCKER_VERSION="${VERSION:-"latest"}" # The Docker/Moby Engine + CLI should match in version
USE_MOBY="${MOBY:-"true"}"
DOCKER_DASH_COMPOSE_VERSION="${DOCKERDASHCOMPOSEVERSION:-"v1"}" # v1 or v2 or none
AZURE_DNS_AUTO_DETECTION="${AZUREDNSAUTODETECTION:-"true"}"
DOCKER_DEFAULT_ADDRESS_POOL="${DOCKERDEFAULTADDRESSPOOL}"
USERNAME="${USERNAME:-"${_REMOTE_USER:-"automatic"}"}"
INSTALL_DOCKER_BUILDX="${INSTALLDOCKERBUILDX:-"true"}"
MICROSOFT_GPG_KEYS_URI="https://packages.microsoft.com/keys/microsoft.asc"
DOCKER_MOBY_ARCHIVE_VERSION_CODENAMES="buster bullseye bionic focal jammy"
DOCKER_LICENSED_ARCHIVE_VERSION_CODENAMES="buster bullseye bionic focal hirsute impish jammy"

# Default: Exit on any failure.
set -e

# Clean up
rm -rf /var/lib/apt/lists/*

# Setup STDERR.
err() {
	echo "(!) $*" >&2
}

if [ "$(id -u)" -ne 0 ]; then
	err 'Script must be run as root. Use sudo, su, or add "USER root" to your Dockerfile before running this script.'
	exit 1
fi

###################
# Helper Functions
# See: https://github.com/microsoft/vscode-dev-containers/blob/main/script-library/shared/utils.sh
###################

# Determine the appropriate non-root user
if [ "${USERNAME}" = "auto" ] || [ "${USERNAME}" = "automatic" ]; then
	USERNAME=""
	POSSIBLE_USERS=("vscode" "node" "codespace" "$(awk -v val=1000 -F ":" '$3==val{print $1}' /etc/passwd)")

	for CURRENT_USER in "${POSSIBLE_USERS[@]}"; do
		if id -u ${CURRENT_USER} > /dev/null 2>&1; then
			USERNAME=${CURRENT_USER}
			break
		fi
	done

	if [ "${USERNAME}" = "" ]; then
		USERNAME=root
	fi
elif [ "${USERNAME}" = "none" ] || ! id -u ${USERNAME} > /dev/null 2>&1; then
	USERNAME=root
fi

# Get central common setting
get_common_setting() {
	if [ "${common_settings_file_loaded}" != "true" ]; then
		curl -sfL "https://aka.ms/vscode-dev-containers/script-library/settings.env" 2>/dev/null -o /tmp/vsdc-settings.env || echo "Could not download settings file. Skipping."
		common_settings_file_loaded=true
	fi

	if [ -f "/tmp/vsdc-settings.env" ]; then
		local multi_line=""
		if [ "$2" = "true" ]; then multi_line="-z"; fi
		local result="$(grep ${multi_line} -oP "$1=\"?\K[^\"]+" /tmp/vsdc-settings.env | tr -d '\0')"
		if [ ! -z "${result}" ]; then declare -g $1="${result}"; fi
	fi

	echo "$1=${!1}"
}

###########################################
# Start docker-in-docker installation
###########################################

# Ensure apt is in non-interactive to avoid prompts
export DEBIAN_FRONTEND=noninteractive


# Source /etc/os-release to get OS info
. /etc/os-release

# Fetch host/container arch.
architecture="$(dpkg --print-architecture)"

# Check if distro is supported
if [ "${USE_MOBY}" = "true" ]; then
	# 'get_common_setting' allows attribute to be updated remotely
	get_common_setting DOCKER_MOBY_ARCHIVE_VERSION_CODENAMES

	if [[ "${DOCKER_MOBY_ARCHIVE_VERSION_CODENAMES}" != *"${VERSION_CODENAME}"* ]]; then
		err "Unsupported  distribution version '${VERSION_CODENAME}'. To resolve, either: (1) set feature option '\"moby\": false' , or (2) choose a compatible OS distribution"
		err "Support distributions include:  ${DOCKER_MOBY_ARCHIVE_VERSION_CODENAMES}"
		exit 1
	fi

	echo "Distro codename  '${VERSION_CODENAME}'  matched filter  '${DOCKER_MOBY_ARCHIVE_VERSION_CODENAMES}'"
else
	get_common_setting DOCKER_LICENSED_ARCHIVE_VERSION_CODENAMES

	if [[ "${DOCKER_LICENSED_ARCHIVE_VERSION_CODENAMES}" != *"${VERSION_CODENAME}"* ]]; then
		err "Unsupported distribution version '${VERSION_CODENAME}'. To resolve, please choose a compatible OS distribution"
		err "Support distributions include:  ${DOCKER_LICENSED_ARCHIVE_VERSION_CODENAMES}"
		exit 1
	fi

	echo "Distro codename  '${VERSION_CODENAME}'  matched filter  '${DOCKER_LICENSED_ARCHIVE_VERSION_CODENAMES}'"
fi

# Set up the necessary apt repos (either Microsoft's or Docker's)
if [ "${USE_MOBY}" = "true" ]; then

	# Name of open source engine/cli
	engine_package_name="moby-engine"
	cli_package_name="moby-cli"

	# Import key safely and import Microsoft apt repo
	get_common_setting MICROSOFT_GPG_KEYS_URI
else
	# Name of licensed engine/cli
	engine_package_name="docker-ce"
	cli_package_name="docker-ce-cli"
fi

# Refresh apt lists
apt-get update

# Soft version matching
if [ "${DOCKER_VERSION}" = "latest" ] || [ "${DOCKER_VERSION}" = "lts" ] || [ "${DOCKER_VERSION}" = "stable" ]; then
	# Empty, meaning grab whatever "latest" is in apt repo
	engine_version_suffix=""
	cli_version_suffix=""
else
	# Fetch a valid version from the apt-cache (eg: the Microsoft repo appends +azure, breakfix, etc...)
	docker_version_dot_escaped="${DOCKER_VERSION//./\\.}"
	docker_version_dot_plus_escaped="${docker_version_dot_escaped//+/\\+}"

	# Regex needs to handle debian package version number format: https://www.systutorials.com/docs/linux/man/5-deb-version/
	docker_version_regex="^(.+:)?${docker_version_dot_plus_escaped}([\\.\\+ ~:-]|$)"
	set +e # Don't exit if finding version fails - will handle gracefully
		cli_version_suffix="=$(apt-cache madison ${cli_package_name} | awk -F"|" '{print $2}' | sed -e 's/^[ \t]*//' | grep -E -m 1 "${docker_version_regex}")"
		engine_version_suffix="=$(apt-cache madison ${engine_package_name} | awk -F"|" '{print $2}' | sed -e 's/^[ \t]*//' | grep -E -m 1 "${docker_version_regex}")"
	set -e

	if [ -z "${engine_version_suffix}" ] || [ "${engine_version_suffix}" = "=" ] || [ -z "${cli_version_suffix}" ] || [ "${cli_version_suffix}" = "=" ] ; then
		err "No full or partial Docker / Moby version match found for \"${DOCKER_VERSION}\" on OS ${ID} ${VERSION_CODENAME} (${architecture}). Available versions:"
		apt-cache madison ${cli_package_name} | awk -F"|" '{print $2}' | grep -oP '^(.+:)?\K.+'
		exit 1
	fi

	echo "engine_version_suffix ${engine_version_suffix}"
	echo "cli_version_suffix ${cli_version_suffix}"
fi

# Uninstall Docker / Moby CLI if not already installed
if type docker > /dev/null 2>&1 && type dockerd > /dev/null 2>&1; then
	set +e # Handle error gracefully

	if [ "${USE_MOBY}" = "true" ]; then
		apt-get -y purge moby-cli${cli_version_suffix} moby-buildx moby-engine${engine_version_suffix}
		apt-get -y purge moby-compose || err "Package moby-compose (Docker Compose v2) not available for OS ${ID} ${VERSION_CODENAME} (${architecture}). Skipping."
	else
		apt-get -y purge docker-ce-cli${cli_version_suffix} docker-ce${engine_version_suffix}
		apt-get -y purge docker-compose-plugin || echo "(*) Package docker-compose-plugin (Docker Compose v2) not available for OS ${ID} ${VERSION_CODENAME} (${architecture}). Skipping."
	fi

	set -e
fi

echo "Finished uninstalling docker / moby!"

# If 'docker-compose' command is to be included
if [ "${DOCKER_DASH_COMPOSE_VERSION}" != "none" ]; then

	# Install Docker Compose if not already installed and is on a supported architecture
	if type docker-compose > /dev/null 2>&1; then
		update-alternatives --remove-all docker-compose
		rm -f /usr/local/bin/docker-compose
	fi
fi

if [ -n "$(grep -e "^docker:" /etc/group)" ]; then
	gpasswd -d ${USERNAME} docker
	groupdel docker
fi

rm -rf "${USERNAME}/.docker"
rm -rf "${_REMOTE_USER_HOME}/.docker"
rm /usr/local/share/docker-init.sh

echo 'uninstall-docker-in-docker script has completed!'
