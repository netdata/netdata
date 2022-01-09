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

When no configuration file is found, the module tries to connect to TCP/IP socket: `localhost:5432`.

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

---

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fcollectors%2Fpython.d.plugin%2Fpostgres%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
