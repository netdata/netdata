# Netdata Agent Configuration

The main Netdata agent configuration is `netdata.conf`.

## The Netdata config directory

On most Linux systems, by using our [recommended one-line installation](https://github.com/netdata/netdata/blob/master/packaging/installer/README.md#install-on-linux-with-one-line-installer), the **Netdata config
directory** will be `/etc/netdata/`. The config directory contains several configuration files with the `.conf` extension, a
few directories, and a shell script named `edit-config`.

> Some operating systems will use `/opt/netdata/etc/netdata/` as the config directory. If you're not sure where yours
> is, navigate to `http://NODE:19999/netdata.conf` in your browser, replacing `NODE` with the IP address or hostname of
> your node, and find the `# config directory = ` setting. The value listed is the config directory for your system.

All of Netdata's documentation assumes that your config directory is at `/etc/netdata`, and that you're running any scripts from inside that directory.


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

