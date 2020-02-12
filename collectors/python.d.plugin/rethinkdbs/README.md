# rethinkdbs

Module monitor rethinkdb health metrics.

Following charts are drawn:

1.  **Connected Servers**

    -   connected
    -   missing

2.  **Active Clients**

    -   active

3.  **Queries** per second

    -   queries

4.  **Documents** per second

    -   documents

## Configuration

Edit the `python.d/rethinkdbs.conf` configuration file using `edit-config` from the your agent's [config
directory](../../../docs/step-by-step/step-04.md#find-your-netdataconf-file), which is typically at `/etc/netdata`.

```bash
cd /etc/netdata   # Replace this path with your Netdata config directory, if different
sudo ./edit-config python.d/rethinkdbs.conf
```

```yaml
localhost:
  name     : 'local'
  host     : '127.0.0.1'
  port     : 28015
  user     : "user"
  password : "pass"
```

When no configuration file is found, module tries to connect to `127.0.0.1:28015`.

---

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fcollectors%2Fpython.d.plugin%2Frethinkdbs%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
