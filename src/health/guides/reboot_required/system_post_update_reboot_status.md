### Understand the alert

*Added in version 2.1.0*

This alert is triggered on Debian-based systems when a reboot is required to complete the update of (system) packages. The corresponding metric is Post-Update Reboot Status (`system.post_update_reboot_status`).

### What does “reboot required” mean?

When installing updated versions of packages, sometimes a reboot of the system is required for the update to take full effect. On Debian-based systems (including Ubuntu), this is marked by the existance of `/var/run/reboot-required`. If the file exists, it will contain the following text that is also shown shown when logging into the machine via the console or ssh:


```*** System restart required ***```

The reboot-required flag can be set in the following scenarios:

- New kernel versions.
- Updates to important libraries like glibc, OpenSSL.
- Updates to long-running processes or their dependencies.
- Security updates in general.

> ⚠️ **Warning**
>
> If systems are never rebooted as standard practice, reboots become _untested_. Should an event occur that causes the system to restart unexpectly, it is then unknown if they will come back up properly!
>
> It is therefore *recommended* to plan for reboots, regularly and/or when the reboot-required flag is set and this alert is triggered.

### Troubleshoot the alert

The most appropriate action is to reboot the (virtual) machine at the earliest convenience. This ensures the operating system and all services run on the latest installed version of their components and dependencies.

In some cases it may be sufficient to restart some services. This does not clear the flag, but may help to plan the appropriate time to schedule the reboot. The packages involved with setting the flag are listed in `/var/run/reboot-required.pkgs`. The flag and this list is managed by the `update-notifier-common` package.

There are several tools to interpret the list of packages to futher understand which processes and services are involved. These all have a different perspective on what to do and may give an incomplete picture.

#### checkrestart

Part of the `debian-goodies` package. This tool scans the running processes and shows a report of its findings.

#### needrestart

In the `needrestart` package. This tool scans the running processes, kernel and microcode, and offers a UI for restarting services when appropriate. It also hooks into the packaging system to restart services automatically.

You can run `needrestart` non-interactively, without performing any actions, with:


```$ sudo DEBIAN_FRONTEND=noninteractive needrestart -n 2>&1 | cat```
<details><summary>example output</summary>

```

Pending kernel upgrade!

Running kernel version:
  5.15.0-67-generic

Diagnostics:
  The currently running kernel version is not the expected kernel version 5.15.0-125-generic.

Restarting the system to load the new kernel will not be handled automatically, so you should consider rebooting. [Return]

Failed to check for processor microcode upgrades.

Services to be restarted:

Service restarts being deferred:
 systemctl restart ModemManager.service
 systemctl restart NetworkManager.service
 systemctl restart accounts-daemon.service
 systemctl restart acpid.service
 systemctl restart apache2.service
 systemctl restart atd.service
 systemctl restart avahi-daemon.service
 systemctl restart colord.service
 systemctl restart cron.service
 /etc/needrestart/restart.d/dbus.service
 systemctl restart gdm.service
 systemctl restart gdm3.service
 systemctl restart irqbalance.service
 systemctl restart kerneloops.service
 systemctl restart memcached.service
 systemctl restart mongodb.service
 systemctl restart networkd-dispatcher.service
 systemctl restart polkit.service
 systemctl restart postfix@-.service
 systemctl restart prometheus-process-exporter.service
 systemctl restart rsyslog.service
 systemctl restart rtkit-daemon.service
 systemctl restart smartmontools.service
 systemctl restart ssh.service
 systemctl restart switcheroo-control.service
 systemctl restart systemd-journald.service
 systemctl restart systemd-journald@netdata.service
 systemctl restart systemd-logind.service
 systemctl restart systemd-resolved.service
 systemctl restart systemd-timesyncd.service
 systemctl restart systemd-udevd.service
 systemctl restart udisks2.service
 systemctl restart unattended-upgrades.service
 systemctl restart unifi.service
 systemctl restart upower.service
 systemctl restart uuidd.service
 systemctl restart whoopsie.service
 systemctl restart zfs-zed.service
 systemctl restart zsysd.service

No containers need to be restarted.

User sessions running outdated binaries:
 gdm @ user manager service: systemd[10233]
 ralphm @ session #264890: ssh-agent[2400801]
 ralphm @ session #269836: ssh-agent[1965630,2158281]
 ralphm @ user manager service: systemd[2243900]
```
</details>


### Useful resources

1. [How to find out if my Ubuntu/Debian Linux server needs a reboot](https://www.cyberciti.biz/faq/how-to-find-out-if-my-ubuntudebian-linux-server-needs-a-reboot/)
