# HAProxy collector

Monitors frontend and backend metrics such as bytes in, bytes out, sessions current, sessions in queue current.
And health metrics such as backend servers status (server check should be used).

Plugin can obtain data from URL or Unix socket.

Requirement:

- Socket must be readable and writable by the `netdata` user.
- URL must have `stats uri <path>` present in the haproxy config, otherwise you will get HTTP 503 in the haproxy logs.

It produces:

1. **Frontend** family charts

    - Kilobytes in/s
    - Kilobytes out/s
    - Sessions current
    - Sessions in queue current

2. **Backend** family charts

    - Kilobytes in/s
    - Kilobytes out/s
    - Sessions current
    - Sessions in queue current

3. **Health** chart

    - number of failed servers for every backend (in DOWN state)

## Configuration

Edit the `python.d/haproxy.conf` configuration file using `edit-config` from the Netdata [config
directory](/docs/netdata-agent/configuration/README.md), which is typically at `/etc/netdata`.

```bash
cd /etc/netdata   # Replace this path with your Netdata config directory, if different
sudo ./edit-config python.d/haproxy.conf
```

Sample:

```yaml
via_url:
  user: 'username' # ONLY IF stats auth is used
  pass: 'password' # # ONLY IF stats auth is used
  url: 'http://ip.address:port/url;csv;norefresh'
```

OR

```yaml
via_socket:
  socket: 'path/to/haproxy/sock'
```

If no configuration is given, module will fail to run.


### Troubleshooting

To troubleshoot issues with the `haproxy` module, run the `python.d.plugin` with the debug option enabled. The 
output will give you the output of the data collection job or error messages on why the collector isn't working.

First, navigate to your plugins directory, usually they are located under `/usr/libexec/netdata/plugins.d/`. If that's 
not the case on your system, open `netdata.conf` and look for the setting `plugins directory`. Once you're in the 
plugin's directory, switch to the `netdata` user.

```bash
cd /usr/libexec/netdata/plugins.d/
sudo su -s /bin/bash netdata
```

Now you can manually run the `haproxy` module in debug mode:

```bash
./python.d.plugin haproxy debug trace
```

