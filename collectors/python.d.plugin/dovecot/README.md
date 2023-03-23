<!--
title: "Dovecot monitoring with Netdata"
custom_edit_url: "https://github.com/netdata/netdata/edit/master/collectors/python.d.plugin/dovecot/README.md"
sidebar_label: "Dovecot"
learn_status: "Published"
learn_topic_type: "References"
learn_rel_path: "Integrations/Monitor/Webapps"
-->

# Dovecot collector

Provides statistics information from Dovecot server.

Statistics are taken from dovecot socket by executing `EXPORT global` command.
More information about dovecot stats can be found on [project wiki page.](http://wiki2.dovecot.org/Statistics)

Module isn't compatible with new statistic api (v2.3), but you are still able to use the module with Dovecot v2.3
by following [upgrading steps.](https://wiki2.dovecot.org/Upgrading/2.3).

**Requirement:**
Dovecot UNIX socket with R/W permissions for user `netdata` or Dovecot with configured TCP/IP socket.

Module gives information with following charts:

1.  **sessions**

    -   active sessions

2.  **logins**

    -   logins

3.  **commands** - number of IMAP commands

    -   commands

4.  **Faults**

    -   minor
    -   major

5.  **Context Switches**

    -   voluntary
    -   involuntary

6.  **disk** in bytes/s

    -   read
    -   write

7.  **bytes** in bytes/s

    -   read
    -   write

8.  **number of syscalls** in syscalls/s

    -   read
    -   write

9.  **lookups** - number of lookups per second

    -   path
    -   attr

10. **hits** - number of cache hits

    -   hits

11. **attempts** - authorization attempts

    -   success
    -   failure

12. **cache** - cached authorization hits

    -   hit
    -   miss

## Configuration

Edit the `python.d/dovecot.conf` configuration file using `edit-config` from the Netdata [config
directory](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md), which is typically at `/etc/netdata`.

```bash
cd /etc/netdata   # Replace this path with your Netdata config directory, if different
sudo ./edit-config python.d/dovecot.conf
```

Sample:

```yaml
localtcpip:
  name     : 'local'
  host     : '127.0.0.1'
  port     : 24242

localsocket:
  name     : 'local'
  socket   : '/var/run/dovecot/stats'
```

If no configuration is given, module will attempt to connect to dovecot using unix socket localized in `/var/run/dovecot/stats`




### Troubleshooting

To troubleshoot issues with the `dovecot` module, run the `python.d.plugin` with the debug option enabled. The 
output will give you the output of the data collection job or error messages on why the collector isn't working.

First, navigate to your plugins directory, usually they are located under `/usr/libexec/netdata/plugins.d/`. If that's 
not the case on your system, open `netdata.conf` and look for the setting `plugins directory`. Once you're in the 
plugin's directory, switch to the `netdata` user.

```bash
cd /usr/libexec/netdata/plugins.d/
sudo su -s /bin/bash netdata
```

Now you can manually run the `dovecot` module in debug mode:

```bash
./python.d.plugin dovecot debug trace
```

