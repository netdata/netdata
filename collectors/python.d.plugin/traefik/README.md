# traefik

Module uses the `health` API to provide statistics.

It produces:

1.  **Responses** by statuses

    -   success (1xx, 2xx, 304)
    -   error (5xx)
    -   redirect (3xx except 304)
    -   bad (4xx)
    -   other (all other responses)

2.  **Responses** by codes

    -   2xx (successful)
    -   5xx (internal server errors)
    -   3xx (redirect)
    -   4xx (bad)
    -   1xx (informational)
    -   other (non-standart responses)

3.  **Detailed Response Codes** requests/s (number of responses for each response code family individually)

4.  **Requests**/s

    -   request statistics

5.  **Total response time**

    -   sum of all response time

6.  **Average response time**

7.  **Average response time per iteration**

8.  **Uptime**

    -   Traefik server uptime

## configuration

Needs only `url` to server's `health`

Here is an example for local server:

```yaml
update_every : 1
priority     : 60000

local:
  url     : 'http://localhost:8080/health'
```

Without configuration, module attempts to connect to `http://localhost:8080/health`.

---

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fcollectors%2Fpython.d.plugin%2Ftraefik%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
