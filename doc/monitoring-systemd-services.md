# Monitoring systemd services

netdata monitors **systemd services**. Example:

![image](https://cloud.githubusercontent.com/assets/2662304/21964372/20cd7b84-db53-11e6-98a2-b9c986b082c0.png)

Support per distribution:

system|systemd services<br/>charts shown|`tree`<br/>`/sys/fs/cgroup`|comments
:-------:|:-------:|:-------:|:------------
Arch Linux|YES| |
Gentoo|NO| |can be enabled, see below
Ubuntu 16.04 LTS|YES| |
Ubuntu 16.10|YES|[here](http://pastebin.com/PiWbQEXy)|
Fedora 25|YES|[here](http://pastebin.com/ax0373wF)|
Debian 8|NO| |can be enabled, see below
AMI|NO|[here](http://pastebin.com/FrxmptjL)|not a systemd system
Centos 7.3.1611|NO|[here](http://pastebin.com/SpzgezAg)|can be enabled, see below

### how to enable cgroup accounting on systemd systems that is by default disabled

You can verify there is no accounting enabled, by running `systemd-cgtop`. The program will show only resources for cgroup ` / `, but all services will show nothing.

To enable cgroup accounting, execute this:

```sh
sed -e 's|^#Default\(.*\)Accounting=.*$|Default\1Accounting=yes|g' /etc/systemd/system.conf >/tmp/system.conf
```

To see the changes it made, run this:

```
# diff /etc/systemd/system.conf /tmp/system.conf
40,44c40,44
< #DefaultCPUAccounting=no
< #DefaultIOAccounting=no
< #DefaultBlockIOAccounting=no
< #DefaultMemoryAccounting=no
< #DefaultTasksAccounting=yes
---
> DefaultCPUAccounting=yes
> DefaultIOAccounting=yes
> DefaultBlockIOAccounting=yes
> DefaultMemoryAccounting=yes
> DefaultTasksAccounting=yes
```

If you are happy with the changes, run:

```sh
# copy the file to the right location
sudo cp /tmp/system.conf /etc/systemd/system.conf

# restart systemd to take it into account
sudo systemctl daemon-reexec
```

(`systemctl daemon-reload` does not reload the configuration of the server - so you have to execute `systemctl daemon-reexec`).

Now, when you run `systemd-cgtop`, services will start reporting usage (if it does not, restart a service - any service - to wake it up). Refresh your netdata dashboard, and you will have the charts too.

In case memory accounting is missing, you will need to enable it at your kernel, by appending the following kernel boot options and rebooting:

```
cgroup_enable=memory swapaccount=1
```

You can add the above, directly at the `linux` line in your `/boot/grub/grub.cfg` or appending them to the `GRUB_CMDLINE_LINUX` in `/etc/default/grub` (in which case you will have to run `update-grub` before rebooting). On DigitalOcean debian images you may have to set it at `/etc/default/grub.d/50-cloudimg-settings.cfg`.
