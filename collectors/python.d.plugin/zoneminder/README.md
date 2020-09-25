# Netdata zoneminder collector
Collector to monitor your zoneminder cameras. 


## Requirements 
1. requests python library: https://requests.readthedocs.io/en/master/
    System wide installation of library is needed, i.e., run:
    ```
    sudo -H python -m pip install requests
    ```
1. PyJWT python library: https://pyjwt.readthedocs.io/en/latest/
    System wide installation of library is needed, i.e., run:
    ```
    sudo -H python -m pip install pyjwt
    ```
1. zoneminder: https://zoneminder.com/
    You need to make sure zoneminder's api is enabled: https://zoneminder.readthedocs.io/en/stable/api.html#enabling-api

## Configuration

Edit the `python.d/zoneminder.conf` configuration file using `edit-config` from the your agent's config
directory, which is typically at `/etc/netdata`.

```bash
cd /etc/netdata   # Replace this path with your Netdata config directory, if different
sudo ./edit-config python.d/zoneminder.conf
```

Example:
```yaml
zm_url: 'http://127.0.0.1/zm'   # the zoneminder URL 
zm_user: ''                     # the zoneminder username to use 
zm_pass: ''                     # the zoneminder password to use
```

"zm_user" and "zm_pass" password can be left blank if authentication is not set. If charts are not shown on netdata dashboard, try restarting netdata:
```
service netdata restart
```

## Charts
This module generates 4 charts:
1. FPS of each camera 
1. Bandwidth of each camera
1. Events of each camera
1. Disk usage of all saved events 

## Alarms
1. Average FPS of last 5 minutes: by default this alarm sets "warn" level when average fps is less than 5 in the last 5 minutes, and "critical" level when average fps is less than 2 in the last 5 minutes. 
1. Time since last successful data collection: "warn" when no data is collected for last 5 minutes, and "critical" when no data is collected for last 15 minutes. 

To edit alarms:
```bash
cd /etc/netdata   # Replace this path with your Netdata config directory, if different
sudo ./edit-config health.d/zoneminder.conf
```


---