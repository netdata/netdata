# tor

Module connects to tor control port to collect traffic statistics.

**Requirements:**

-   `tor` program
-   `stem` python package

It produces only one chart:

1.  **Traffic**

    -   read
    -   write

## configuration

Needs only `control_port`

Here is an example for local server:

```yaml
update_every : 1
priority     : 60000

local_tcp:
 name: 'local'
 control_port: 9051

local_socket:
 name: 'local'
 control_port: '/var/run/tor/control'
```

### prerequisite

Add to `/etc/tor/torrc`:

```
ControlPort 9051
```

For more options please read the manual.

Without configuration, module attempts to connect to `127.0.0.1:9051`.

---

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fcollectors%2Fpython.d.plugin%2Ftor%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
