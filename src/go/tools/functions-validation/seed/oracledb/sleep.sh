#!/usr/bin/env bash
set -euo pipefail

ORA_USER="${ORA_USER:-netdata}"
ORA_PASS="${ORA_PASS:-Netdata123!}"
ORA_HOST="${ORA_HOST:-oracledb}"
ORA_PORT="${ORA_PORT:-1521}"
ORA_SERVICE="${ORA_SERVICE:-FREEPDB1}"

sqlplus -s "${ORA_USER}/${ORA_PASS}@//${ORA_HOST}:${ORA_PORT}/${ORA_SERVICE}" @/seed/sleep.sql
