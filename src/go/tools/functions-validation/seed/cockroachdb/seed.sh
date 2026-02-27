#!/usr/bin/env sh
set -e

cockroach sql --insecure --host=cockroachdb:26258 -f /seed/seed.sql
