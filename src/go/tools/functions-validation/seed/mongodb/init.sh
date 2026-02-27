#!/usr/bin/env bash
set -euo pipefail

host="${MONGO_HOST:-mongo}"
user="${MONGO_INITDB_ROOT_USERNAME:-root}"
pass="${MONGO_INITDB_ROOT_PASSWORD:-rootpw}"

mongosh --quiet "mongodb://${user}:${pass}@${host}:27017/admin" /seed/init.js
