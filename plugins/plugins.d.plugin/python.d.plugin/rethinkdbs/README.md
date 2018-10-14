# rethinkdbs

Module monitor rethinkdb health metrics.

Following charts are drawn:

1. **Connected Servers**
 * connected
 * missing

2. **Active Clients**
 * active

3. **Queries** per second
 * queries

4. **Documents** per second
 * documents

### configuration

```yaml

localhost:
  name     : 'local'
  host     : '127.0.0.1'
  port     : 28015
  user     : "user"
  password : "pass"
```

When no configuration file is found, module tries to connect to `127.0.0.1:28015`.

---
