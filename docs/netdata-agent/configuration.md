# Netdata Agent Configuration

The main Netdata agent configuration is `netdata.conf`.

## edit `netdata.conf`

To edit `netdata.conf`, run this on your terminal:

```bash
cd /etc/netdata 2>/dev/null || cd /opt/netdata/etc/netdata
sudo ./edit-config netdata.conf
```

Your editor will open.

## downloading `netdata.conf`

The running version of `netdata.conf` can be downloaded from a running Netdata agent, at this URL:

```
http://agent-ip:19999/netdata.conf
```

You can save and use this version, using these commands:

```bash
cd /etc/netdata 2>/dev/null || cd /opt/netdata/etc/netdata
curl -ksSLo /tmp/netdata.conf.new http://localhost:19999/netdata.conf && sudo mv -i /tmp/netdata.conf.new netdata.conf 
```
