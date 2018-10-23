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
