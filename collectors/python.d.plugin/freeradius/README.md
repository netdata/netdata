# freeradius

Uses the `radclient` command to provide freeradius statistics. It is not recommended to run it every second.

It produces:

1. **Authentication counters:**
 * access-accepts
 * access-rejects
 * auth-dropped-requests
 * auth-duplicate-requests
 * auth-invalid-requests
 * auth-malformed-requests
 * auth-unknown-types

2. **Accounting counters:** [optional]
 * accounting-requests
 * accounting-responses
 * acct-dropped-requests
 * acct-duplicate-requests
 * acct-invalid-requests
 * acct-malformed-requests
 * acct-unknown-types

3. **Proxy authentication counters:** [optional]
 * proxy-access-accepts
 * proxy-access-rejects
 * proxy-auth-dropped-requests
 * proxy-auth-duplicate-requests
 * proxy-auth-invalid-requests
 * proxy-auth-malformed-requests
 * proxy-auth-unknown-types

4. **Proxy accounting counters:** [optional]
 * proxy-accounting-requests
 * proxy-accounting-responses
 * proxy-acct-dropped-requests
 * proxy-acct-duplicate-requests
 * proxy-acct-invalid-requests
 * proxy-acct-malformed-requests
 * proxy-acct-unknown-typesa


### configuration

Sample:

```yaml
local:
  host       : 'localhost'
  port       : '18121'
  secret     : 'adminsecret'
  acct       : False # Freeradius accounting statistics.
  proxy_auth : False # Freeradius proxy authentication statistics.
  proxy_acct : False # Freeradius proxy accounting statistics.
```

**Freeradius server configuration:**

The configuration for the status server is automatically created in the sites-available directory.
By default, server is enabled and can be queried from every client.
FreeRADIUS will only respond to status-server messages, if the status-server virtual server has been enabled.

To do this, create a link from the sites-enabled directory to the status file in the sites-available directory:
 * cd sites-enabled
 * ln -s ../sites-available/status status

and restart/reload your FREERADIUS server.

---

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fcollectors%2Fpython.d.plugin%2Ffreeradius%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)]()
