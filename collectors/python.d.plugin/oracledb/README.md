<!--
title: "OracleDB monitoring with Netdata"
custom_edit_url: "https://github.com/netdata/netdata/edit/master/collectors/python.d.plugin/oracledb/README.md"
sidebar_label: "OracleDB"
learn_status: "Published"
learn_topic_type: "References"
learn_rel_path: "Integrations/Monitor/Databases"
-->

# OracleDB collector

Monitors the performance and health metrics of the Oracle database.

## Requirements

-   `oracledb` package.

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

1.  Install `oracledb` package ([link](https://python-oracledb.readthedocs.io/en/latest/user_guide/installation.html)).

3.  Create a read-only `netdata` user with proper access to your Oracle Database Server.

Connect to your Oracle database with an administrative user and execute:

```SQL
CREATE USER netdata IDENTIFIED BY <PASSWORD>;

GRANT CONNECT TO netdata;
GRANT SELECT_CATALOG_ROLE TO netdata;
```

3.  Get or create a [data source name](https://python-oracledb.readthedocs.io/en/latest/user_guide/connection_handling.html#connection-strings) for your database.

## Configuration

Edit the `python.d/oracledb.conf` configuration file using `edit-config` from the Netdata [config
directory](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md), which is typically at `/etc/netdata`.

```bash
cd /etc/netdata   # Replace this path with your Netdata config directory, if different
sudo ./edit-config python.d/oracledb.conf
```

```yaml
local:
  user: 'netdata'
  password: 'secret'
  dsn: '(description= (retry_count=20)(retry_delay=3)(address=(protocol=tcps)(port=1521)(host=adb.us-ashburn-1.oraclecloud.com))(connect_data=(service_name=sdf98789f98sfs98f_projectid_low.adb.oraclecloud.com))(security=(ssl_server_dn_match=yes)))'


remote:
  user: 'netdata'
  password: 'secret'
  dsn: '(description= (retry_count=20)(retry_delay=3)(address=(protocol=tcps)(port=1521)(host=adb.us-ashburn-1.oraclecloud.com))(connect_data=(service_name=sdf98789f98sfs98f_projectid_low.adb.oraclecloud.com))(security=(ssl_server_dn_match=yes)))'
```

All parameters are required. Without them module will fail to start.


### Troubleshooting

To troubleshoot issues with the `oracledb` module, run the `python.d.plugin` with the debug option enabled. The 
output will give you the output of the data collection job or error messages on why the collector isn't working.

First, navigate to your plugins directory, usually they are located under `/usr/libexec/netdata/plugins.d/`. If that's 
not the case on your system, open `netdata.conf` and look for the setting `plugins directory`. Once you're in the 
plugin's directory, switch to the `netdata` user.

```bash
cd /usr/libexec/netdata/plugins.d/
sudo su -s /bin/bash netdata
```

Now you can manually run the `oracledb` module in debug mode:

```bash
./python.d.plugin oracledb debug trace
```

