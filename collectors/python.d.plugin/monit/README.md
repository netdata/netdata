# monit

Monit monitoring module. Data is grabbed from stats XML interface (exists for a long time, but not mentioned in official documentation). Mostly this plugin shows statuses of monit targets, i.e. [statuses of specified checks](https://mmonit.com/monit/documentation/monit.html#Service-checks).

1.  **Filesystems**

    -   Filesystems
    -   Directories
    -   Files
    -   Pipes

2.  **Applications**

    -   Processes (+threads/childs)
    -   Programs

3.  **Network**

    -   Hosts (+latency)
    -   Network interfaces

## configuration

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

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fcollectors%2Fpython.d.plugin%2Fmonit%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
