# squid

This module will monitor one or more squid instances depending on configuration.

It produces following charts:

1. **Client Bandwidth** in kilobits/s
 * in
 * out
 * hits

2. **Client Requests** in requests/s
 * requests
 * hits
 * errors

3. **Server Bandwidth** in kilobits/s
 * in
 * out

4. **Server Requests** in requests/s
 * requests
 * errors

### configuration

```yaml
priority     : 50000

local:
  request : 'cache_object://localhost:3128/counters'
  host    : 'localhost'
  port    : 3128
```

Without any configuration module will try to autodetect where squid presents its `counters` data

---
