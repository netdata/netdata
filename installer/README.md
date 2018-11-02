# Installation
![image10](https://cloud.githubusercontent.com/assets/2662304/14253729/534c6f9c-fa95-11e5-8243-93eb0df719aa.gif)


## Linux Package Managers

You can install the latest release of netdata, using your package manager in

   - Arch Linux (`sudo pacman -S netdata`)
   - Alpine Linux (`sudo apk add netdata`)
   - Debian Linux (`sudo apt install netdata`)
   - Gentoo Linux (`sudo emerge --ask netdata`)
   - OpenSUSE (`sudo zypper install netdata`)
   - Solus Linux (`sudo eopkg install netdata`)
   - Ubuntu Linux >= 18.04 (`sudo apt install netdata`)

  For security and portability reasons, this is the preferred installation method.

## Linux one liner

![](http://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.requests_per_url&options=unaligned&dimensions=kickstart&group=sum&after=-3600&label=last+hour&units=installations&value_color=orange&precision=0) ![](http://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.requests_per_url&options=unaligned&dimensions=kickstart&group=sum&after=-86400&label=today&units=installations&precision=0)

To install netdata from source to your systems and keep it up to date automatically, run the following:

:hash:**`bash <(curl -Ss https://my-netdata.io/kickstart.sh)`**

(do not `sudo` this command, it will do it by itself as needed)

The command:

1. detects the distro and **installs the required system packages** for building netdata (will ask for confirmation)
2. downloads the latest netdata source tree to `/usr/src/netdata.git`.
3. installs netdata by running `./netdata-installer.sh` from the source tree.
4. installs `netdata-updater.sh` to `cron.daily`, so your netdata installation will be updated daily (you will get a message from cron only if the update fails).

The `kickstart.sh` script passes all its parameters to `netdata-installer.sh`, so you can add more parameters to change the installation directory, enable/disable plugins, etc (check below).

For automated installs, append a space + `--dont-wait` to the command line. You can also append `--dont-start-it` to prevent the installer from starting netdata. Example:

```sh
bash <(curl -Ss https://my-netdata.io/kickstart.sh) all --dont-wait --dont-start-it
```
## Pre-built static binary for Linux 64bit

You can install a pre-compiled static binary of netdata for any Intel/AMD 64bit Linux system (even those that don't have a package manager, like CoreOS, CirrOS, busybox systems, etc). You can also use these packages on systems with broken or unsupported package managers.

<br/>![](http://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.requests_per_url&options=unaligned&dimensions=kickstart64&group=sum&after=-3600&label=last+hour&units=installations&value_color=orange&precision=0) ![](http://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.requests_per_url&options=unaligned&dimensions=kickstart64&group=sum&after=-86400&label=today&units=installations&precision=0)

To install netdata with a binary package on any Linux distro, any kernel version - for **Intel/AMD 64bit** hosts, run the following:

:hash:&nbsp; **`bash <(curl -Ss https://my-netdata.io/kickstart-static64.sh)`**

(do not `sudo` this command, it will do it by itself as needed; the target system does not need `bash` installed, check below for instructions to run it without `bash`)

*Note: The static builds install netdata at `/opt/netdata`*

For automated installs, append a space + `--dont-wait` to the command line. You can also append `--dont-start-it` to prevent the installer from starting netdata. Example:

```sh
bash <(curl -Ss https://my-netdata.io/kickstart-static64.sh) --dont-wait --dont-start-it
```

If your shell fails to handle the above one liner, do this:

```sh
# download the script with curl
curl https://my-netdata.io/kickstart-static64.sh >/tmp/kickstart-static64.sh

# or, download the script with wget
wget -O /tmp/kickstart-static64.sh https://my-netdata.io/kickstart-static64.sh

# run the downloaded script (any sh is fine, no need for bash)
sh /tmp/kickstart-static64.sh
```

The static binary files are kept in repo [binary-packages](https://github.com/netdata/binary-packages). You can download any of the `.run` files, and run it. These files are self-extracting shell scripts built with [makeself](https://github.com/megastep/makeself). The target system does **not** need to have bash installed. The same files can be used for updates too.

- [Linux, install from source, by hand](#linux-by-hand)<br/>semi-automatic, with more details about the steps involved and actions taken.

Non-Linux:

- [Install from package or source, on FreeBSD](#freebsd)
- [Install from package, on pfSense](#pfsense)
- [Enable netdata on FreeNAS Corral](#freenas)
- [Install from package or source, on macOS (OS X)](#macos)



### Linux by hand

To install the latest git version of netdata, please follow these 2 steps:

1. [Prepare your system](#1-prepare-your-system)

   Install the required packages on your system.

2. [Install netdata](#2-install-netdata)

   Download and install netdata. You can also update it the same way.

---

### 1. Prepare your system

Try our experimental automatic requirements installer (no need to be root). This will try to find the packages that should be installed on your system to build and run netdata. It supports most major Linux distributions released after 2010:

- **Alpine** Linux and its derivatives (you have to install `bash` yourself, before using the installer)
- **Arch** Linux and its derivatives
- **Gentoo** Linux and its derivatives
- **Debian** Linux and its derivatives (including **Ubuntu**, **Mint**)
- **Fedora** and its derivatives (including **Red Hat Enterprise Linux**, **CentOS**, **Amazon Machine Image**)
- **SuSe** Linux and its derivatives (including **openSuSe**)
- **SLE12** Must have your system registered with Suse Customer Center or have the DVD. See [#1162](https://github.com/netdata/netdata/issues/1162)

Install the packages for having a **basic netdata installation** (system monitoring and many applications, without  `mysql` / `mariadb`, `postgres`, `named`, hardware sensors and `SNMP`):

```sh
curl -Ss 'https://raw.githubusercontent.com/netdata/netdata-demo-site/master/install-required-packages.sh' >/tmp/kickstart.sh && bash /tmp/kickstart.sh -i netdata
```

Install all the required packages for **monitoring everything netdata can monitor**:

```sh
curl -Ss 'https://raw.githubusercontent.com/netdata/netdata-demo-site/master/install-required-packages.sh' >/tmp/kickstart.sh && bash /tmp/kickstart.sh -i netdata-all
```

If the above do not work for you, please [open a github issue](https://github.com/netdata/netdata/issues/new?title=packages%20installer%20failed&labels=installation%20help&body=The%20experimental%20packages%20installer%20failed.%0A%0AThis%20is%20what%20it%20says:%0A%0A%60%60%60txt%0A%0Aplease%20paste%20your%20screen%20here%0A%0A%60%60%60) with a copy of the message you get on screen. We are trying to make it work everywhere (this is also why the script [reports back](https://github.com/netdata/netdata/issues/2054) success or failure for all its runs).

---

This is how to do it by hand:

```sh
# Debian / Ubuntu
apt-get install zlib1g-dev uuid-dev libmnl-dev gcc make git autoconf autoconf-archive autogen automake pkg-config curl

# Fedora
dnf install zlib-devel libuuid-devel libmnl-devel gcc make git autoconf autoconf-archive autogen automake pkgconfig curl findutils

# CentOS / Red Hat Enterprise Linux
yum install autoconf automake curl gcc git libmnl-devel libuuid-devel lm_sensors make MySQL-python nc pkgconfig python python-psycopg2 PyYAML zlib-devel

```

Please note that for RHEL/CentOS you might need [EPEL](http://www.tecmint.com/how-to-enable-epel-repository-for-rhel-centos-6-5/).

Once netdata is compiled, to run it the following packages are required (already installed using the above commands):

package|description
:-----:|-----------
`libuuid`|part of `util-linux` for GUIDs management
`zlib`|gzip compression for the internal netdata web server

*netdata will fail to start without the above.*

netdata plugins and various aspects of netdata can be enabled or benefit when these are installed (they are optional):

package|description
:-----:|-----------
`bash`|for shell plugins and **alarm notifications**
`curl`|for shell plugins and **alarm notifications**
`iproute` or `iproute2`|for monitoring **Linux traffic QoS**<br/>use `iproute2` if `iproute` reports as not available or obsolete
`python`|for most of the external plugins
`python-yaml`|used for monitoring **beanstalkd**
`python-beanstalkc`|used for monitoring **beanstalkd**
`python-dnspython`|used for monitoring DNS query time
`python-ipaddress`|used for monitoring **DHCPd**<br/>this package is required only if the system has python v2. python v3 has this functionality embedded
`python-mysqldb`<br/>or<br/>`python-pymysql`|used for monitoring **mysql** or **mariadb** databases<br/>`python-mysqldb` is a lot faster and thus preferred
`python-psycopg2`|used for monitoring **postgresql** databases
`python-pymongo`|used for monitoring **mongodb** databases
`nodejs`|used for `node.js` plugins for monitoring **named** and **SNMP** devices
`lm-sensors`|for monitoring **hardware sensors**
`libmnl`|for collecting netfilter metrics
`netcat`|for shell plugins to collect metrics from remote systems

*netdata will greatly benefit if you have the above packages installed, but it will still work without them.*

---

# 2. Install netdata

Do this to install and run netdata:

```sh

# download it - the directory 'netdata' will be created
git clone https://github.com/netdata/netdata.git --depth=1
cd netdata

# run script with root privileges to build, install, start netdata
./netdata-installer.sh

```

* If you don't want to run it straight-away, add `--dont-start-it` option.

* If you don't want to install it on the default directories, you can run the installer like this: `./netdata-installer.sh --install /opt`. This one will install netdata in `/opt/netdata`.

Once the installer completes, the file `/etc/netdata/netdata.conf` will be created (if you changed the installation directory, the configuration will appear in that directory too).

You can edit this file to set options. One common option to tweak is `history`, which controls the size of the memory database netdata will use. By default is `3600` seconds (an hour of data at the charts) which makes netdata use about 10-15MB of RAM (depending on the number of charts detected on your system). Check **[[Memory Requirements]]**.

To apply the changes you made, you have to restart netdata.

## starting netdata at boot

In the `system` directory you can find scripts and configurations for the various distros.

#### systemd

The installer already installs `netdata.service` if it detects a systemd system.

To install `netdata.service` by hand, run:

```sh
# stop netdata
killall netdata

# copy netdata.service to systemd
cp system/netdata.service /etc/systemd/system/

# let systemd know there is a new service
systemctl daemon-reload

# enable netdata at boot
systemctl enable netdata

# start netdata
systemctl start netdata
```

#### init.d

In the system directory you can find `netdata-lsb`. Copy it to the proper place according to your distribution documentation. For Ubuntu, this can be done via running the following commands as root.

```sh
# copy the netdata startup file to /etc/init.d
cp system/netdata-lsb /etc/init.d/netdata

# make sure it is executable
chmod +x /etc/init.d/netdata

# enable it
update-rc.d netdata defaults
```

#### openrc (gentoo)

In the `system` directory you can find `netdata-openrc`. Copy it to the proper place according to your distribution documentation.

#### CentOS / Red Hat Enterprise Linux

For older versions of RHEL/CentOS that don't have systemd, an init script is included in the system directory. This can be installed by running the following commands as root.

```sh
# copy the netdata startup file to /etc/init.d
cp system/netdata-init-d /etc/init.d/netdata

# make sure it is executable
chmod +x /etc/init.d/netdata

# enable it
chkconfig --add netdata
```

_There have been some recent work on the init script, see PR https://github.com/netdata/netdata/pull/403_

#### other systems

You can start netdata by running it from `/etc/rc.local` or equivalent.

## log-rotation

The installer, when run as `root`, will install `/etc/logrotate.d/netdata`.

## Updating netdata after its installation

### Manual update

#### Method 1: netdata-updater.sh

`netdata-installer.sh` generates `netdata-updater.sh` upon any successful installation  
You can use this script to update your netdata installation with the same options you used to install it in the first place.

```sh
# go to the git downloaded directory
cd /path/to/git/downloaded/netdata

# run the updater
./netdata-updater.sh
```

_Netdata will be restarted with the new version._

#### Method 2: git pull

You can also update netdata to the latest version by hand, using this:

```sh
# go to the git downloaded directory
cd /path/to/git/downloaded/netdata

# download the latest version
git pull

# rebuild it, install it, run it
./netdata-installer.sh
```

_Netdata will be restarted with the new version._

### Auto-update

_Please, consider the risks of running an auto-update. Something can always go wrong. Keep an eye on your installation, and run a manual update if something ever fails._

You can call `netdata-updater.sh` from a cron-job. A successful update will not trigger an email from cron. 

```sh
# Edit your cron-jobs
crontab -e

# add a cron-job at the bottom. This one will update netdata every day at 6:00AM:
# update netdata
0 6 * * * /path/to/git/downloaded/netdata/netdata-updater.sh
```

---

## Working with netdata

- You can start netdata by executing it with `/usr/sbin/netdata` (the installer will also start it).

- You can stop netdata by killing it with `killall netdata`.
    You can stop and start netdata at any point. Netdata saves on exit its round robbin
    database to `/var/cache/netdata` so that it will continue from where it stopped the last time.

Access to the web site, for all graphs, is by default on port `19999`, so go to:

 ```
 http://127.0.0.1:19999/
 ```

You can get the running config file at any time, by accessing `http://127.0.0.1:19999/netdata.conf`.

---

## Uninstalling netdata

#### netdata was installed from source (or `kickstart.sh`)

The script `netdata-installer.sh` generates another script called `netdata-uninstaller.sh`.

To uninstall netdata, run:

```
cd /path/to/netdata.git
./netdata-uninstaller.sh --force
```

The uninstaller will ask you to confirm all deletions.

#### netdata was installed with `kickstart-static64.sh` package

Stop netdata with one of the following:

- `service netdata stop` (non-systemd systems)
- `systemctl stop netdata` (systemd systems)

Disable running netdata at startup, with one of the following (based on your distro):

- `rc-update del netdata`
- `update-rc.d netdata disable`
- `chkconfig netdata off`
- `systemctl disable netdata`

Delete the netdata files:

1. `rm -rf /opt/netdata`
2. `groupdel netdata`
3. `userdel netdata`
4. `rm /etc/logrotate.d/netdata`
5. `rm /lib/systemd/system/netdata.service` or `rm /etc/init.d/netdata`, depending on the distro.

---

## Other Systems

We are trying to collect all the information about netdata package maintainers at [issue 651](https://github.com/netdata/netdata/issues/651). So, please have a look there for ASUSTOR NAS, OpenWRT, ReadyNAS, etc.

##### FreeBSD

You can install netdata from ports or packages collection.

This is how to install the latest netdata version from sources on FreeBSD:

```sh
# install required packages
pkg install bash e2fsprogs-libuuid git curl autoconf automake pkgconf pidof

# download netdata
git clone https://github.com/netdata/netdata.git --depth=1

# install netdata in /opt/netdata
cd netdata
./netdata-installer.sh --install /opt
```

##### pfSense
To install netdata on pfSense run the following commands (within a shell or under Diagnostics/Command Prompt within the pfSense web interface).

Change platform (i386/amd64, etc) and FreeBSD versions (10/11, etc) according to your environment and change netdata version (1.10.0 in example) according to latest version present within the FreeSBD repository:-

Note first three packages are downloaded from the pfSense repository for maintaining compatibility with pfSense, netdata is downloaded from the FreeBSD repository.
```
pkg install pkgconf
pkg install bash
pkg install e2fsprogs-libuuid
pkg add http://pkg.freebsd.org/FreeBSD:11:amd64/latest/All/netdata-1.10.0.txz
```
To start netdata manually run `service netdata onestart`

To start netdata automatically at each boot add `service netdata start` as a Shellcmd within the pfSense web interface (under **Services/Shellcmd**, which you need to install beforehand under **System/Package Manager/Available Packages**).
Shellcmd Type should be set to `Shellcmd`. 
![](https://user-images.githubusercontent.com/36808164/36930790-4db3aa84-1f0d-11e8-8752-cdc08bb7207c.png)
Alternatively more information can be found in https://doc.pfsense.org/index.php/Installing_FreeBSD_Packages, for achieving the same via the command line and scripts.  

If you experience an issue with `/usr/bin/install` absense on pfSense 2.3 or earlier, update pfSense or use workaround from [https://redmine.pfsense.org/issues/6643](https://redmine.pfsense.org/issues/6643)

##### FreeNAS
On FreeNAS-Corral-RELEASE (>=10.0.3), netdata is pre-installed. 

To use netdata, the service will need to be enabled and started from the FreeNAS **[CLI](https://github.com/freenas/cli)**.

To enable the netdata service:
```
service netdata config set enable=true
```

To start the netdata service:
```
service netdata start
```

##### macOS

netdata on macOS still has limited charts, but external plugins do work.

You can either install netdata with [Homebrew](https://brew.sh/)

```sh
brew install netdata
```

or from source:

```sh
# install Xcode Command Line Tools
xcode-select --install
```
click `Install` in the software update popup window, then
```sh
# install HomeBrew package manager
/usr/bin/ruby -e "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/master/install)"

# install required packages
brew install ossp-uuid autoconf automake pkg-config

# download netdata
git clone https://github.com/netdata/netdata.git --depth=1

# install netdata in /usr/local/netdata
cd netdata
sudo ./netdata-installer.sh --install /usr/local
```

The installer will also install a startup plist to start netdata when your Mac boots.

##### Alpine 3.x

Execute these commands to install netdata in Alpine Linux 3.x:

```
# install required packages
apk add alpine-sdk bash curl zlib-dev util-linux-dev libmnl-dev gcc make git autoconf automake pkgconfig python logrotate

# if you plan to run node.js netdata plugins
apk add nodejs

# download netdata - the directory 'netdata' will be created
git clone https://github.com/netdata/netdata.git --depth=1
cd netdata


# build it, install it, start it
./netdata-installer.sh


# make netdata start at boot
echo -e "#!/usr/bin/env bash\n/usr/sbin/netdata" >/etc/local.d/netdata.start
chmod 755 /etc/local.d/netdata.start

# make netdata stop at shutdown
echo -e "#!/usr/bin/env bash\nkillall netdata" >/etc/local.d/netdata.stop
chmod 755 /etc/local.d/netdata.stop

# enable the local service to start automatically
rc-update add local
```

##### Synology

The documentation previously recommended installing the Debian Chroot package from the Synology community package sources and then running netdata from within the chroot. This does not work, as the chroot environment does not have access to `/proc`, and therefore exposes very few metrics to netdata. Additionally, [this issue](https://github.com/SynoCommunity/spksrc/issues/2758), still open as of 2018/06/24, indicates that the Debian Chroot package is not suitable for DSM versions greater than version 5 and may corrupt system libraries and render the NAS unable to boot. 

The good news is that the 64-bit static installer works fine if your NAS is one that uses the amd64 architecture. It will install the content into `/opt/netdata`, making future removal safe and simple.

###### Additional Work

When netdata is first installed, it will run as _root_. This may or may not be acceptable for you, and since other installations run it as the _netdata_ user, you might wish to do the same. This requires some extra work:

1. Creat a group `netdata` via the Synology group interface. Give it no access to anything.
2. Create a user `netdata` via the Synology user interface. Give it no access to anything and a random password. Assign the user to the `netdata` group. Netdata will chuid to this user when running.
3. Change ownership of the following directories, as defined in [Netdata Security](https://github.com/netdata/netdata/wiki/netdata-security):

```
$ chown -R root:netdata /opt/netdata/usr/share/netdata
$ chown -R netdata:netdata /opt/netdata/var/lib/netdata /opt/netdata/var/cache/netdata
$ chown -R netdata:root /opt/netdata/var/log/netdata
```

Additionally, as of 2018/06/24, the netdata installer doesn't recognize DSM as an operating system, so no init script is installed. You'll have to do this manually:

1. Add [this file](https://gist.github.com/oskapt/055d474d7bfef32c49469c1b53e8225f) as `/etc/rc.netdata`. Make it executable with `chmod 0755 /etc/rc.netdata`.
2. Edit `/etc/rc.local` and add a line calling `/etc/rc.netdata` to have it start on boot:

```
# Netdata startup
[ -x /etc/rc.netdata ] && /etc/rc.netdata start
```
