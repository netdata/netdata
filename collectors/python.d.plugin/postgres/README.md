<!--
title: "PostgreSQL monitoring with Netdata"
custom_edit_url: https://github.com/netdata/netdata/edit/master/collectors/python.d.plugin/postgres/README.md
sidebar_label: "PostgreSQL"
-->

# PostgreSQL monitoring with Netdata

Collects database health and performance metrics.

## Requirements

-   `python-psycopg2` package. You have to install it manually and make sure that it is available to the `netdata` user, either using `pip`, the package manager of your Linux distribution, or any other method you prefer.

-   PostgreSQL v9.4+

Following charts are drawn:

1.  **Database size** MB

    -   size

2.  **Current Backend Processes** processes

    -   active

3.  **Current Backend Process Usage** percentage

    -   used
    -   available

4.  **Write-Ahead Logging Statistics** files/s

    -   total
    -   ready
    -   done

5.  **Checkpoints** writes/s

    -   scheduled
    -   requested

6.  **Current connections to db** count

    -   connections

7.  **Tuples returned from db** tuples/s

    -   sequential
    -   bitmap

8.  **Tuple reads from db** reads/s

    -   disk
    -   cache

9.  **Transactions on db** transactions/s

    -   committed
    -   rolled back

10.  **Tuples written to db** writes/s

    -   inserted
    -   updated
    -   deleted
    -   conflicts

11. **Locks on db** count per type

    -   locks

12. **Standby delta** KB

    - sent delta
    - write delta
    - flush delta
    - replay delta

13. **Standby lag** seconds

    - write lag
    - flush lag
    - replay lag

14. **Average number of blocking transactions in db** processes

    - blocking

## Configuration

Edit the `python.d/postgres.conf` configuration file using `edit-config` from the Netdata [config
directory](/docs/configure/nodes.md), which is typically at `/etc/netdata`.

```bash
cd /etc/netdata   # Replace this path with your Netdata config directory, if different
sudo ./edit-config python.d/postgres.conf
```

When no configuration file is found, the module tries to connect to TCP/IP socket: `localhost:5432` with the 
following collection jobs.

```yaml
socket:
  name         : 'socket'
  user         : 'postgres'
  database     : 'postgres'

tcp:
  name         : 'tcp'
  user         : 'postgres'
  database     : 'postgres'
  host         : 'localhost'
  port         : 5432
```

**Note**: Every job collection must have a unique identifier. In cases that you monitor multiple DBs, every
job must have it's own name. Use a mnemonic of your preference (e.g us_east_db, us_east_tcp)

## Troubleshooting

To troubleshoot issues with the `postgres` collector, run the `python.d.plugin` with the debug option enabled. The output
should give you clues as to why the collector isn't working.

First, navigate to your plugins directory, usually at `/usr/libexec/netdata/plugins.d/`. If that's not the case on your
system, open `netdata.conf` and look for the setting `plugins directory`. Once you're in the plugin's directory, switch
to the `netdata` user.

```bash
cd /usr/libexec/netdata/plugins.d/
sudo su -s /bin/bash netdata
```

You can now run the `python.d.plugin` to debug the collector:

```bash
./python.d.plugin postgres debug trace
```

---


