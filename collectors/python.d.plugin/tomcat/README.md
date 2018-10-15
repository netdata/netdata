# tomcat

Present tomcat containers memory utilization.

Charts:

1. **Requests** per second
 * accesses

2. **Volume** in KB/s
 * volume

3. **Threads**
 * current
 * busy

4. **JVM Free Memory** in MB
 * jvm

### configuration

```yaml
localhost:
  name : 'local'
  url  : 'http://127.0.0.1:8080/manager/status?XML=true'
  user : 'tomcat_username'
  pass : 'secret_tomcat_password'
```

Without configuration, module attempts to connect to `http://localhost:8080/manager/status?XML=true`, without any credentials.
So it will probably fail.

---
