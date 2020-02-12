# ovpn_status_log

Module monitor openvpn-status log file.

**Requirements:**

-   If you are running multiple OpenVPN instances out of the same directory, MAKE SURE TO EDIT DIRECTIVES which create output files
    so that multiple instances do not overwrite each other's output files.

-   Make sure NETDATA USER CAN READ openvpn-status.log

-   Update_every interval MUST MATCH interval on which OpenVPN writes operational status to log file.

It produces:

1.  **Users** OpenVPN active users

    -   users

2.  **Traffic** OpenVPN overall bandwidth usage in kilobit/s

    -   in
    -   out

## Configuration

Edit the `python.d/ovpn_status_log.conf` configuration file using `edit-config` from the your agent's [config
directory](../../../docs/step-by-step/step-04.md#find-your-netdataconf-file), which is typically at `/etc/netdata`.

```bash
cd /etc/netdata   # Replace this path with your Netdata config directory, if different
sudo ./edit-config python.d/ovpn_status_log.conf
```

Sample:

```yaml
default
 log_path     : '/var/log/openvpn-status.log'
```

---

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fcollectors%2Fpython.d.plugin%2Fovpn_status_log%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
