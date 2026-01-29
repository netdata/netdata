#!/usr/bin/env bash
set -euo pipefail

PROXY_HOST="${PROXY_HOST:-proxysql}"
ADMIN_USER="${ADMIN_USER:-admin}"
ADMIN_PASS="${ADMIN_PASS:-admin}"
BACKEND_HOST="${BACKEND_HOST:-mysql}"
BACKEND_PORT="${BACKEND_PORT:-3306}"

for i in $(seq 1 30); do
  if mysql -h "$PROXY_HOST" -P 6032 -u "$ADMIN_USER" -p"$ADMIN_PASS" -e "SELECT 1" > /dev/null 2>&1; then
    break
  fi
  sleep 2
done

mysql -h "$PROXY_HOST" -P 6032 -u "$ADMIN_USER" -p"$ADMIN_PASS" <<SQL
UPDATE global_variables SET variable_value='admin:admin;netdata:netdata' WHERE variable_name='admin-admin_credentials';
UPDATE global_variables SET variable_value='0.0.0.0:6032' WHERE variable_name='admin-mysql_ifaces';
LOAD ADMIN VARIABLES TO RUNTIME;
SAVE ADMIN VARIABLES TO DISK;
DELETE FROM mysql_servers;
INSERT INTO mysql_servers(hostgroup_id, hostname, port) VALUES (0, '$BACKEND_HOST', $BACKEND_PORT);
DELETE FROM mysql_users;
INSERT INTO mysql_users(username, password, default_hostgroup) VALUES ('netdata', 'netdata', 0);
LOAD MYSQL SERVERS TO RUNTIME;
SAVE MYSQL SERVERS TO DISK;
LOAD MYSQL USERS TO RUNTIME;
SAVE MYSQL USERS TO DISK;
SQL

mysql -h "$PROXY_HOST" -P 6033 -u netdata -pnetdata <<SQL
CREATE DATABASE IF NOT EXISTS netdata;
CREATE TABLE IF NOT EXISTS netdata.t (id INT PRIMARY KEY, v VARCHAR(10));
INSERT INTO netdata.t (id, v) VALUES (1, 'a') ON DUPLICATE KEY UPDATE v='a';
INSERT INTO netdata.t (id, v) VALUES (2, 'b') ON DUPLICATE KEY UPDATE v='b';
SELECT * FROM netdata.t WHERE id > 0;
SQL

sleep 2
