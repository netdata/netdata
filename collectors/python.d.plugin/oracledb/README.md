<!--
title: "OracleDB monitoring with Netdata"
custom_edit_url: https://github.com/netdata/netdata/edit/master/collectors/python.d.plugin/oracledb/README.md
sidebar_label: "OracleDB"
-->

# OracleDB monitoring with Netdata

Monitors the performance and health metrics of the Oracle database.

## Requirements

-   `cx_Oracle` package.
-   Oracle Client (using `cx_Oracle` requires Oracle Client libraries to be installed).

It produces following charts:

-   session activity
    -   Session Count
    -   Session Limit Usage
    -   Logons
-   disk activity
    -   Physical Disk Reads/Writes
    -   Sorts On Disk
    -   Full Table Scans
-   database and buffer activity
    -   Database Wait Time Ratio
    -   Shared Pool Free Memory
    -   In-Memory Sorts Ratio
    -   SQL Service Response Time
    -   User Rollbacks
    -   Enqueue Timeouts
-   cache
    -   Cache Hit Ratio
    -   Global Cache Blocks Events
-   activities
    -   Activities
-   wait time
    -   Wait Time
-   tablespace
    -   Size
    -   Usage
    -   Usage In Percent
-   allocated space
    -   Size
    -   Usage
    -   Usage In Percent

## prerequisite

To use the Oracle module do the following:

1.  Install `cx_Oracle` package ([link](https://cx-oracle.readthedocs.io/en/latest/user_guide/installation.html)).

2.  Install Oracle Client libraries
    ([link](https://cx-oracle.readthedocs.io/en/latest/user_guide/installation.html#install-oracle-client)).

3.  Create a read-only `netdata` user with proper access to your Oracle Database Server.

Connect to your Oracle database with an administrative user and execute:

```
ALTER SESSION SET "_ORACLE_SCRIPT"=true;

CREATE USER netdata IDENTIFIED BY <PASSWORD>;

GRANT CONNECT TO netdata;
GRANT SELECT_CATALOG_ROLE TO netdata;
```

## Configuration

Edit the `python.d/oracledb.conf` configuration file using `edit-config` from the Netdata [config
directory](/docs/configure/nodes.md), which is typically at `/etc/netdata`.

```bash
cd /etc/netdata   # Replace this path with your Netdata config directory, if different
sudo ./edit-config python.d/oracledb.conf
```

```yaml
local:
  user: 'netdata'
  password: 'secret'
  server: 'localhost:1521'
  service: 'XE'

remote:
  user: 'netdata'
  password: 'secret'
  server: '10.0.0.1:1521'
  service: 'XE'
```

All parameters are required. Without them module will fail to start.


