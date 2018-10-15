# fail2ban

Module monitor fail2ban log file to show all bans for all active jails

**Requirements:**
 * fail2ban.log file MUST BE readable by netdata (A good idea is to add  **create 0640 root netdata** to fail2ban conf at logrotate.d)

It produces one chart with multiple lines (one line per jail)

### configuration

Sample:

```yaml
local:
 log_path: '/var/log/fail2ban.log'
 conf_path: '/etc/fail2ban/jail.local'
 exclude: 'dropbear apache'
```
If no configuration is given, module will attempt to read log file at `/var/log/fail2ban.log` and conf file at `/etc/fail2ban/jail.local`.
If conf file is not found default jail is `ssh`.

---
