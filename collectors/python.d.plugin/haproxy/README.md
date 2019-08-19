# haproxy

Module monitors frontend and backend metrics such as bytes in, bytes out, sessions current, sessions in queue current.
And health metrics such as backend servers status (server check should be used).

Plugin can obtain data from url **OR** unix socket.

**Requirement:**
Socket MUST be readable AND writable by the `netdata` user.

It produces:

1.  **Frontend** family charts

    -   Kilobytes in/s
    -   Kilobytes out/s
    -   Sessions current
    -   Sessions in queue current

2.  **Backend** family charts

    -   Kilobytes in/s
    -   Kilobytes out/s
    -   Sessions current
    -   Sessions in queue current

3.  **Health** chart

    -   number of failed servers for every backend (in DOWN state)

## configuration

Sample:

```yaml
via_url:
  user       : 'username' # ONLY IF stats auth is used
  pass       : 'password' # # ONLY IF stats auth is used
  url     : 'http://ip.address:port/url;csv;norefresh'
```

OR

```yaml
via_socket:
  socket       : 'path/to/haproxy/sock'
```

If no configuration is given, module will fail to run.

---

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fcollectors%2Fpython.d.plugin%2Fhaproxy%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
