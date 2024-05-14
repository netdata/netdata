# Running a Local Dashboard through Cloudflare Tunnels

## Summary of tasks

- Make a `netdata-web` HTTP tunnel on the parent node, so the web interface can be viewed publicly
- Make a `netdata-tcp` tcp tunnel on the parent node, so it can receive tcp streams from child nodes
- Provide access to the `netdata-tcp` tunnel on the child nodes, so you can send the tcp stream to the parent node
- Make sure the parent node uses port `19999` for both web and tcp streams
- Make sure that the child nodes have `mode = none` in the `[web]` section of the `netdata.conf` file, and `destination = tcp:127.0.0.1:19999` in the `[stream]` section of the `stream.conf` file

## Detailed instructions with commands and service files

- Install the `cloudflared` package on all your Netdata nodes, follow the repository instructions [here](https://pkg.cloudflare.com/index.html)

- Login to cloudflare with `sudo cloudflared login` on all your Netdata nodes

### Parent node: public web interface and receiving stats from Child nodes

- Create the HTTP tunnel  
    `sudo cloudflared tunnel create netdata-web`
- Start routing traffic  
    `sudo cloudflared tunnel route dns netdata-web netdata-web.my.domain`
- Create a service by making a file called `/etc/systemd/system/cf-tun-netdata-web.service` and input:

```ini
[Unit]
Description=cloudflare tunnel netdata-web
After=network-online.target

[Service]
Type=simple
User=root
Group=root
ExecStart=/usr/bin/cloudflared --no-autoupdate tunnel run --url http://localhost:19999 netdata-web
Restart=on-failure
TimeoutStartSec=0
RestartSec=5s

[Install]
WantedBy=multi-user.target
```

- Create the TCP tunnel  
  `sudo cloudflared tunnel create netdata-tcp`
- Start routing traffic  
  `sudo cloudflared tunnel route dns netdata-tcp netdata-tcp.my.domain`
- Create a service by making a file called `/etc/systemd/system/cf-tun-netdata-tcp.service` and input:

```ini
[Unit]
Description=cloudflare tunnel netdata-tcp
After=network-online.target

[Service]
Type=simple
User=root
Group=root
ExecStart=/usr/bin/cloudflared --no-autoupdate tunnel run --url tcp://localhost:19999 netdata-tcp
Restart=on-failure
TimeoutStartSec=0
RestartSec=5s

[Install]
WantedBy=multi-user.target
```

### Child nodes: send stats to the Parent node

- Create a service by making a file called `/etc/systemd/system/cf-acs-netdata-tcp.service` and input:

```ini
[Unit]
Description=cloudflare access netdata-tcp
After=network-online.target

[Service]
Type=simple
User=root
Group=root
ExecStart=/usr/bin/cloudflared --no-autoupdate access tcp --url localhost:19999 --tunnel-host netdata-tcp.my.domain
Restart=on-failure
TimeoutStartSec=0
RestartSec=5s

[Install]
WantedBy=multi-user.target
```

You can edit the configuration file using the `edit-config` script from the Netdata [config directory](https://github.com/netdata/netdata/blob/master/docs/netdata-agent/configuration.md#the-netdata-config-directory).

- Edit `netdata.conf` and input:

```ini
[web]
    mode = none
```

- Edit `stream.conf` and input:

```ini
[stream]
    destination = tcp:127.0.0.1:19999
```

[Restart the Agents](https://github.com/netdata/netdata/blob/master/packaging/installer/README.md#maintaining-a-netdata-agent-installation), and you are done!

You should now be able to have a Local Dashboard that gets its metrics from Child instances, running through Cloudflare tunnels.

> ### Note
>
> You can find the origin of this page in [this discussion](https://discord.com/channels/847502280503590932/1154164395799216189/1154556625944854618) in our Discord server.
>
> We thought it was going to be helpful to all users, so we included it in our docs.
