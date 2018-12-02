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
Verify that user netdata can execute `chronyc tracking`. If necessary, update `/etc/chrony.conf`, `cmdallow`.

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
