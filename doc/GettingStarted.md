# Getting Started

These are your first steps **after** you have installed netdata. If you haven't installed it already, please check the [installation page](../installer).

## Accessing the dashboard

To access the netdata dashboard, navigate with your browser to:

```
http://your.server.ip:19999/
```

<details markdown="1"><summary>Click here, if it does not work.</summary>

**Verify Netdata is running.**

Open an ssh session to the server and execute `sudo ps -e netdata`. It should respond with the PID of the netdata daemon. If it prints nothing, Netdata is not running. Check the [installation page](../installer) to install it.

**Verify Netdata responds to HTTP requests.**

Using the same ssh session, execute `curl -Ss http://localhost:19999`. It should dump on your screen the `index.html` page of the dashboard. If it does not, check the [installation page](../installer) to install it.

**Verify Netdata receives the HTTP requests.**

On the same ssh session, execute `tail -f /var/log/netdata/access.log` (if you installed the static 64bit package, use: `tail -f /opt/netdata/var/log/netdata/access.log`). This command will print on your screen all HTTP requests Netdata receives.

Next, try to access the dashboard using your web browser, using the URL posted above. If nothing is printed on your terminal, the HTTP request is not routed to your Netdata.

If you are not sure about your server IP, run this for a hint: `ip route get 8.8.8.8 | grep -oP " src [0-9\.]+ "`. It should print the IP of your server.

If still Netdata does not receive the requests, something is blocking them. A firewall possibly. Please check your network.

</details>&nbsp;<br/>

When you install multiple Netdata servers, all your servers will appear at the `my-netdata` menu at the top left of the dashboard. For this to work, you have to manually access just once, the dashboard of each of your netdata servers.

The `my-netdata` menu is more than just browser bookmarks. When switching Netdata servers from that menu, any settings of the current view are propagated to the other netdata server:

- the current charts panning (drag the charts left or right),
- the current charts zooming (`SHIFT` + mouse wheel over a chart),
- the highlighted time-frame (`ALT` + select an area on a chart),
- the scrolling position of the dashboard,
- the theme you use,
- etc.

are all sent over to other netdata server, to allow you troubleshoot cross-server performance issues easily.

## Starting and stopping Netdata

Netdata installer integrates Netdata to your init / systemd environment.

To start/stop Netdata, depending on your environment, you should use:

- `systemctl start netdata` and `systemctl stop netdata`
- `service netdata start` and `service netdata stop`
- `/etc/init.d/netdata start` and `/etc/init.d/netdata stop`

Once netdata is installed, the installer configures it to start at boot and stop at shutdown.

For more information about using these commands, consult your system documentation.

## Sizing Netdata

The default installation of netdata provides a small round-robin database, for just 1 hour of data. Depending on the memory your system has and the amount you can dedicate to Netdata, you should adapt this. On production systems with limited RAM, we suggest to set this to 3-4 hours. For best results you should set this to 24 or 48 hours.

For every hour of data, Netdata currently needs about 25MB of RAM. So, if you can dedicate about 100MB of RAM to netdata, you should set its database size to 4 hours.

To do this, edit `/etc/netdata/netdata.conf` (or `/opt/netdata/etc/netdata/netdata.conf`) and set:

```
[global]
    history = SECONDS
```

Make sure the `history` line is not commented (comment lines start with `#`).

1 hour is 3600 seconds, so the number you need to set is the result of `HOURS * 3600`.

!!! danger
    Be careful when you set this on production systems. If you set it too high, your system may run out of memory. By default, netdata is configured to be killed first when the system starves for memory, but better be careful to avoid issues.

For more information about Netdata memory requirements, [check this page](../database).

## Service discovery and auto-detection



## Enabling and disabling plugins

