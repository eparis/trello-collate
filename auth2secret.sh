#!/bin/bash

set -e
set -u

auth=$(cat auth.yaml | base64 --wrap=0)

echo "apiVersion: v1
data:
  auth: ${auth}
kind: Secret
type: Opaque
metadata:
  name: trello-auth
  namespace: default" > secret.yaml
