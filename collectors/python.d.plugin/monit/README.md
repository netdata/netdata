# monit

Monit monitoring module. Data is grabbed from stats XML interface (exists for a long time, but not mentioned in official documentation). Mostly this plugin shows statuses of monit targets, i.e. [statuses of specified checks](https://mmonit.com/monit/documentation/monit.html#Service-checks).

1. **Filesystems**
 * Filesystems
 * Directories
 * Files
 * Pipes

2. **Applications**
 * Processes (+threads/childs)
 * Programs

3. **Network**
 * Hosts (+latency)
 * Network interfaces

### configuration

Sample:

```yaml
local:
  name     : 'local'
  url     : 'http://localhost:2812'
  user:    : admin
  pass:    : monit
```

If no configuration is given, module will attempt to connect to monit as `http://localhost:2812`.

---
