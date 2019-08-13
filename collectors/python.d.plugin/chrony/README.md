# chrony

This module monitors the precision and statistics of a local chronyd server.

It produces:

* frequency
* last offset
* RMS offset
* residual freq
* root delay
* root dispersion
* skew
* system time

**Requirements:**
Verify that user Netdata can execute `chronyc tracking`. If necessary, update `/etc/chrony.conf`, `cmdallow`.

### Configuration

Sample:
```yaml
# data collection frequency:
update_every: 1

# chrony query command:
local:
  command: 'chronyc -n tracking'
```

---

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fcollectors%2Fpython.d.plugin%2Fchrony%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)]()
