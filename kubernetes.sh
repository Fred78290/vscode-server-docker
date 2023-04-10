#!/bin/bash

if [ ! -f auth ]; then
	htpasswd -c auth vscode
fi

cat <<EOF | kubectl apply -f -
$(cat kubernetes.yaml)
---
$(kubectl create secret generic basic-auth -n vscode-server --from-file=auth --dry-run=client -o yaml)
EOF
