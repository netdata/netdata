# Reload health configuration

## Unix systems

You do not need to restart the Netdata Agent between changes to health configuration files, such as specific health entities. Instead, use `netdatacli` and the `reload-health` option to prevent gaps in metrics collection.

```bash
sudo netdatacli reload-health
```

If `netdatacli` doesn't work on your system, send a `SIGUSR2` signal to the daemon, which reloads health configuration
without restarting the entire process.

```bash
killall -USR2 netdata
```
