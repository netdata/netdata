# Running a Local Dashboard through Cloudflare Tunnels

## Summary of tasks

- Make a `netdata-web` HTTP tunnel on the parent node, so the web interface can be viewed publicly
- Make a `netdata-tcp` tcp tunnel on the parent node, so it can receive tcp streams from child nodes
- Provide access to the `netdata-tcp` tunnel on the child nodes, so you can send the tcp stream to the parent node
- Make sure the parent node uses port `19999` for both web and tcp streams
- Make sure that the child nodes have `mode = none` in the `[web]` section of the `netdata.conf` file, and `destination = tcp:127.0.0.1:19999` in the `[stream]` section of the `stream.conf` file
- Bind the parent node's dashboard to `127.0.0.1` so the Cloudflare Tunnel is the only way to reach it, instead of also being exposed directly on the parent's public or LAN interface

:::note

Netdata's local dashboard serves plain **HTTP** on port `19999` and does not terminate TLS by default. This is why `https://localhost:19999` does not work — the browser attempts a TLS handshake against a plain HTTP server. When connecting directly to the Agent, use `http://localhost:19999`.

Because of this, the cloudflared tunnel origin **must** be `http://localhost:19999`, not `https://`. Cloudflare terminates TLS at its edge, so you still reach the dashboard over HTTPS through your tunnel hostname (`https://netdata-web.my.domain`), while the connection between `cloudflared` and the Agent stays plain HTTP.

:::

## Detailed instructions with commands and service files

- Install the `cloudflared` package on all your Netdata nodes, follow the repository instructions [here](https://pkg.cloudflare.com/index.html)

- Login to cloudflare with `sudo cloudflared login` on all your Netdata nodes

### Parent node: public web interface and receiving stats from Child nodes

:::note

By default, Netdata's web server binds to all network interfaces (`bind to = *` in the `[web]` section of `netdata.conf`), so the dashboard would also be directly reachable at the parent's real IP address on port `19999`, alongside the Cloudflare Tunnel. To make the tunnel the only way to reach the dashboard, edit `netdata.conf` on the parent (using the `edit-config` script from the Netdata [config directory](/docs/netdata-agent/configuration/README.md#locate-your-config-directory)) and set:

```ini
[web]
    bind to = 127.0.0.1
```

[Restart the Agent](/docs/netdata-agent/start-stop-restart.md) after this change.

:::

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

You can edit the configuration file using the `edit-config` script from the Netdata [config directory](/docs/netdata-agent/configuration/README.md#locate-your-config-directory).

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

[Restart the Agents](/docs/netdata-agent/start-stop-restart.md), and you are done!

You should now be able to have a Local Dashboard that gets its metrics from Child instances, running through Cloudflare tunnels.

:::note

You can find the origin of this page in [this discussion](https://discord.com/channels/847502280503590932/1154164395799216189/1154556625944854618) in our Discord server.

We thought it was going to be helpful to all users, so we included it in our docs.

:::
