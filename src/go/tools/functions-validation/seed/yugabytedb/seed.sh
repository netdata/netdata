#!/usr/bin/env sh
set -e

/home/yugabyte/bin/ysqlsh -h yugabytedb -U yugabyte -d yugabyte -f /seed/seed.sql
