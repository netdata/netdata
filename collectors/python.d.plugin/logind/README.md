<!--
title: "systemd-logind monitoring with Netdata"
custom_edit_url: https://github.com/netdata/netdata/edit/master/collectors/python.d.plugin/logind/README.md
sidebar_label: "systemd-logind"
-->

# Systemd-Logind monitoring with Netdata

Monitors counts of sessions and users as reported by the `org.freedesktop.login1` DBus API (provided by `systemd-logind` or `elogind`).

It provides the following charts:

1.  **Sessions By Type** Tracks the total number of sessions by session type.

    -   Graphical: Local graphical sessions (running one of X11, Mir, or Wayland)
    -   Console: Local console sessions (`tty` sessions in Logind terms)
    -   Remote: Remote sessions.
    -   Other: Other sessions that do not fit the above types (for example, a session for a cron job or systemd timer unit).

2.  **Sessions By State** Tracks the total number of sessions by session state.

    -   Online: Sessions that are logged in, but not in the foreground.
    -   Active: Sessions that are logged in and in the foreground.
    -   Closing: Sessions that are in the process of logging out.

3.  **Users** Tracks the total number of users by user state.

    -   Offline: Users that are not logged in, but are tacked by logind.
    -   Lingering: Users that are logged out, but have associated services still running.
    -   Online: Users that are logged in but do not have an associated active session.
    -   Active: Users that are logged in and have an associated active session.
    -   Closing: Users that are in the process of logging out and are not lingering.

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

This module needs no configuration. Just make sure the `netdata` user has the required permissions to query the
`org.freedesktop.login1` DBus interface (this should be the case by default on most systems).

## Notes

-   This module requires the `dbus-python` Python bindings for DBus to run. On many systems that use Logind this
    will already be installed. If it is not, you will need to install it (in most cases, the package name will be
    something along the lines of `dbus-python` or `python-dbus`)

-   This module's ability to track logins is dependent on what PAM services
    are configured to register sessions with logind.  In particular, for most systems, it will only track TTY
    logins, local desktop logins, and logins through remote shell connections.

-   Both `systemd-logind` and `elogind` track users by username, not by UID. In most cases this should not matter, but
    if you have multiple users who share a UID for some reason, they will be treated as separate users by this
    collector.
