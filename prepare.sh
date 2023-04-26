#!/bin/bash
set -e

export DEBIAN_FRONTEND=noninteractive

apt update
apt dist-upgrade -y
apt install -y tini

apt autoclean
apt-get clean -y
rm -rf  /usr/share/doc /usr/share/doc-base /var/lib/apt/lists/*

