# litespeed

Module monitor litespeed web server performance metrics.

It produces:

1. **Network Throughput HTTP** in kilobits/s
 * in
 * out

2. **Network Throughput HTTPS** in kilobits/s
 * in
 * out

3. **Connections HTTP** in connections
 * free
 * used

4. **Connections HTTPS** in connections
 * free
 * used

5. **Requests** in requests/s
 * requests

6. **Requests In Processing** in requests
 * processing

7. **Public Cache Hits** in hits/s
 * hits

8. **Private Cache Hits** in hits/s
 * hits

9. **Static Hits** in hits/s
 * hits


### configuration
```yaml
local:
  path  : 'PATH'
```

If no configuration is given, module will use "/tmp/lshttpd/".

---
