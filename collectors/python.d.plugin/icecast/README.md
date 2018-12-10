# icecast

This module will monitor number of listeners for active sources.

**Requirements:**
 * icecast version >= 2.4.0

It produces the following charts:

1. **Listeners** in listeners
 * source number

### configuration

Needs only `url` to server's `/status-json.xsl`

Here is an example for remote server:

```yaml
remote:
  url      : 'http://1.2.3.4:8443/status-json.xsl'
```

Without configuration, module attempts to connect to `http://localhost:8443/status-json.xsl`

---

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fcollectors%2Fpython.d.plugin%2Ficecast%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)]()
