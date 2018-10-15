# apache

This module will monitor one or more Apache servers depending on configuration.

**Requirements:**
 * apache with enabled `mod_status`

It produces the following charts:

1. **Requests** in requests/s
 * requests

2. **Connections**
 * connections

3. **Async Connections**
 * keepalive
 * closing
 * writing

4. **Bandwidth** in kilobytes/s
 * sent

5. **Workers**
 * idle
 * busy

6. **Lifetime Avg. Requests/s** in requests/s
 * requests_sec

7. **Lifetime Avg. Bandwidth/s** in kilobytes/s
 * size_sec

8. **Lifetime Avg. Response Size** in bytes/request
 * size_req

### configuration

Needs only `url` to server's `server-status?auto`

Here is an example for 2 servers:

```yaml
update_every : 10
priority     : 90100

local:
  url      : 'http://localhost/server-status?auto'
  retries  : 20

remote:
  url          : 'http://www.apache.org/server-status?auto'
  update_every : 5
  retries      : 4
```

Without configuration, module attempts to connect to `http://localhost/server-status?auto`

---
