#!/usr/bin/env bash
set -euo pipefail

CH_HOST="${CH_HOST:-clickhouse}"
CH_USER="${CH_USER:-default}"
CH_PASSWORD="${CH_PASSWORD:-netdata}"

clickhouse-client --host "$CH_HOST" --user "$CH_USER" --password "$CH_PASSWORD" --query "CREATE DATABASE IF NOT EXISTS netdata"
clickhouse-client --host "$CH_HOST" --user "$CH_USER" --password "$CH_PASSWORD" --query "CREATE TABLE IF NOT EXISTS netdata.test (id UInt64, value String) ENGINE=MergeTree() ORDER BY id"
clickhouse-client --host "$CH_HOST" --user "$CH_USER" --password "$CH_PASSWORD" --query "INSERT INTO netdata.test VALUES (1,'a'), (2,'b'), (3,'c')"
clickhouse-client --host "$CH_HOST" --user "$CH_USER" --password "$CH_PASSWORD" --query "SELECT count() FROM netdata.test"
clickhouse-client --host "$CH_HOST" --user "$CH_USER" --password "$CH_PASSWORD" --query "SELECT * FROM netdata.test WHERE id > 0"

sleep 1
