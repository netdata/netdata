# ovpn_status_log

Module monitor openvpn-status log file.

**Requirements:**

 * If you are running multiple OpenVPN instances out of the same directory, MAKE SURE TO EDIT DIRECTIVES which create output files
 so that multiple instances do not overwrite each other's output files.

 * Make sure NETDATA USER CAN READ openvpn-status.log

 * Update_every interval MUST MATCH interval on which OpenVPN writes operational status to log file.

It produces:

1. **Users** OpenVPN active users
 * users

2. **Traffic** OpenVPN overall bandwidth usage in kilobit/s
 * in
 * out

### configuration

Sample:

```yaml
default
 log_path     : '/var/log/openvpn-status.log'
```

---
