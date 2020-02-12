# portcheck

Module monitors a remote TCP service.

Following charts are drawn per host:

1.  **Latency** ms

    -   Time required to connect to a TCP port.
    Displays latency in 0.1 ms resolution. If the connection failed, the value is missing.

2.  **Status** boolean

    -   Connection successful
    -   Could not create socket: possible DNS problems
    -   Connection refused: port not listening or blocked
    -   Connection timed out: host or port unreachable

## Configuration

Edit the `python.d/portcheck.conf` configuration file using `edit-config` from the your agent's [config
directory](../../../docs/step-by-step/step-04.md#find-your-netdataconf-file), which is typically at `/etc/netdata`.

```bash
cd /etc/netdata   # Replace this path with your Netdata config directory, if different
sudo ./edit-config python.d/portcheck.conf
```

```yaml
server:
  host: 'dns or ip'     # required
  port: 22              # required
  timeout: 1            # optional
  update_every: 1       # optional
```

### notes

-   The error chart is intended for alarms, badges or for access via API.
-   A system/service/firewall might block Netdata's access if a portscan or
    similar is detected.
-   Currently, the accuracy of the latency is low and should be used as reference only.

---

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fcollectors%2Fpython.d.plugin%2Fportcheck%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
