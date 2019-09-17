# httpcheck

Module monitors remote http server for availability and response time.

Following charts are drawn per job:

1.  **Response time** ms

    -   Time in 0.1 ms resolution in which the server responds.
    If the connection failed, the value is missing.

2.  **Status** boolean

    -   Connection successful
    -   Unexpected content: No Regex match found in the response
    -   Unexpected status code: Do we get 500 errors?
    -   Connection failed: port not listening or blocked
    -   Connection timed out: host or port unreachable

## configuration

Sample configuration and their default values.

```yaml
server:
  url: 'http://host:port/path'  # required
  status_accepted:              # optional
    - 200
  timeout: 1                    # optional, supports decimals (e.g. 0.2)
  update_every: 3               # optional
  regex: 'REGULAR_EXPRESSION'   # optional, see https://docs.python.org/3/howto/regex.html
  redirect: yes                 # optional
```

### notes

-   The status chart is primarily intended for alarms, badges or for access via API.
-   A system/service/firewall might block Netdata's access if a portscan or
    similar is detected.
-   This plugin is meant for simple use cases. Currently, the accuracy of the
    response time is low and should be used as reference only.

---

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fcollectors%2Fpython.d.plugin%2Fhttpcheck%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
