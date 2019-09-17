# postgres

Module monitors one or more postgres servers.

**Requirements:**

-   `python-psycopg2` package. You have to install it manually.

Following charts are drawn:

1.  **Database size** MB

    -   size

2.  **Current Backend Processes** processes

    -   active

3.  **Write-Ahead Logging Statistics** files/s

    -   total
    -   ready
    -   done

4.  **Checkpoints** writes/s

    -   scheduled
    -   requested

5.  **Current connections to db** count

    -   connections

6.  **Tuples returned from db** tuples/s

    -   sequential
    -   bitmap

7.  **Tuple reads from db** reads/s

    -   disk
    -   cache

8.  **Transactions on db** transactions/s

    -   committed
    -   rolled back

9.  **Tuples written to db** writes/s

    -   inserted
    -   updated
    -   deleted
    -   conflicts

10. **Locks on db** count per type

    -   locks

## configuration

For all available options please see module [configuration file](postgres.conf).

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

When no configuration file is found, module tries to connect to TCP/IP socket: `localhost:5432`.

---

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fcollectors%2Fpython.d.plugin%2Fpostgres%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
