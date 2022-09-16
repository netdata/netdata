<!--
title: "systemd-logind monitoring with Netdata"
custom_edit_url: https://github.com/netdata/netdata/edit/master/collectors/python.d.plugin/logind/README.md
sidebar_label: "systemd-logind"
-->

# Systemd-Logind monitoring with Netdata

Monitors active sessions, users, and seats tracked by `systemd-logind` or `elogind`.

It provides the following charts:

1.  **Sessions** Tracks the total number of sessions.

    -   Graphical: Local graphical sessions (running X11, or Wayland, or something else).
    -   Console: Local console sessions.
    -   Remote: Remote sessions.

2.  **Users** Tracks total number of unique user logins of each type.

    -   Graphical
    -   Console
    -   Remote

3.  **Seats** Total number of seats in use.

    -   Seats

## Enable the collector

The `logind` collector is disabled by default. To enable it, use `edit-config` from the Netdata [config
directory](/docs/configure/nodes.md), which is typically at `/etc/netdata`, to edit the `python.d.conf` file.

```bash
cd /etc/netdata   # Replace this path with your Netdata config directory, if different
sudo ./edit-config python.d.conf
```

Change the value of the `logind` setting to `yes`. Save the file and restart the Netdata Agent with `sudo systemctl
restart netdata`, or the appropriate method for your system, to finish enabling the `logind` collector.

## Configuration

This module needs no configuration. Just make sure the `netdata` user
can run the `loginctl` command and get a session list without having to
specify a path.

This will work with any command that can output data in the _exact_
same format as `loginctl list-sessions --no-legend`.  If you have some
other command you want to use that outputs data in this format, you can
specify it using the `command` key like so:

```yaml
command: '/path/to/other/command'
```

Edit the `python.d/logind.conf` configuration file using `edit-config` from the Netdata [config
directory](/docs/configure/nodes.md), which is typically at `/etc/netdata`.

```bash
cd /etc/netdata   # Replace this path with your Netdata config directory, if different
sudo ./edit-config python.d/logind.conf
```

## Notes

-   This module's ability to track logins is dependent on what PAM services
    are configured to register sessions with logind.  In particular, for
    most systems, it will only track TTY logins, local desktop logins,
    and logins through remote shell connections.

-   The users chart counts _usernames_ not UID's.  This is potentially
    important in configurations where multiple users have the same UID.

-   The users chart counts any given user name up to once for _each_ type
    of login.  So if the same user has a graphical and a console login on a
    system, they will show up once in the graphical count, and once in the
    console count.

-   Because the data collection process is rather expensive, this plugin
    is currently disabled by default, and needs to be explicitly enabled in
    `/etc/netdata/python.d.conf` before it will run.

---


