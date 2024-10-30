# Reload health configuration

## Unix systems

No need to restart the Netdata Agent after modifying health configuration files (alerts). Use `netdatacli` to avoid metric collection gaps.

```bash
sudo netdatacli reload-health
```
