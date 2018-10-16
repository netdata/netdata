# phpfpm

This module will monitor one or more php-fpm instances depending on configuration.

**Requirements:**
 * php-fpm with enabled `status` page
 * access to `status` page via web server

It produces following charts:

1. **Active Connections**
 * active
 * maxActive
 * idle

2. **Requests** in requests/s
 * requests

3. **Performance**
 * reached
 * slow

### configuration

Needs only `url` to server's `status`

Here is an example for local instance:

```yaml
update_every : 3
priority     : 90100

local:
  url     : 'http://localhost/status'
  retries : 10
```

Without configuration, module attempts to connect to `http://localhost/status`

---
