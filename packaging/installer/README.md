# Installation

Netdata is a **monitoring agent**. It is designed to be installed and run on all your systems: **physical** and **virtual** servers, **containers**, even **IoT**.

The best way to install Netdata is directly from source. Our **automatic installer** will install any required system packages and compile Netdata directly on your systems.

!!! warning
    You can find Netdata packages distributed by third parties. In many cases, these packages are either too old or broken. So, the suggested ways to install Netdata are the ones in this page.
    **We are currently working to provide our binary packages for all Linux distros.** Stay tuned...

1. [Automatic one line installation](#one-line-installation), easy installation from source, **this is the default**
2. [Install pre-built static binary on any 64bit Linux](#linux-64bit-pre-built-static-binary)
3. [Run Netdata in a docker container](#run-netdata-in-a-docker-container)
4. [Manual installation, step by step](#install-netdata-on-linux-manually)
5. [Install on FreeBSD](#freebsd)
6. [Install on pfSense](#pfsense)
7. [Enable on FreeNAS Corral](#freenas)
8. [Install on macOS (OS X)](#macos)
9. [Install on a Kubernetes cluster](https://github.com/netdata/helmchart#netdata-helm-chart-for-kubernetes-deployments)

See also the list of Netdata [package maintainers](../maintainers) for ASUSTOR NAS, OpenWRT, ReadyNAS, etc.

---

## One line installation

> This method is **fully automatic on all Linux** distributions. FreeBSD and MacOS systems need some preparations before installing Netdata for the first time. Check the [FreeBSD](#freebsd) and the [MacOS](#macos) sections for more information.

To install Netdata from source and keep it up to date automatically, run the following:

```bash
bash <(curl -Ss https://my-netdata.io/kickstart.sh)
```

*(do not `sudo` this command, it will do it by itself as needed)*

![](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.requests_per_url&options=unaligned&dimensions=kickstart&group=sum&after=-3600&label=last+hour&units=installations&value_color=orange&precision=0) ![](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.requests_per_url&options=unaligned&dimensions=kickstart&group=sum&after=-86400&label=today&units=installations&precision=0)

<details markdown="1"><summary>Click here for more information and advanced use of this command.</summary>

&nbsp;<br/>
Verify the integrity of the script with this:

```bash
[ "4e339c2e66192c122484476ef48b6843" = "$(curl -Ss https://my-netdata.io/kickstart.sh | md5sum | cut -d ' ' -f 1)" ] && echo "OK, VALID" || echo "FAILED, INVALID"
```
*It should print `OK, VALID` if the script is the one we ship.*

The `kickstart.sh` script:

- detects the Linux distro and **installs the required system packages** for building Netdata (will ask for confirmation)
- downloads the latest Netdata source tree to `/usr/src/netdata.git`.
- installs Netdata by running `./netdata-installer.sh` from the source tree.
- installs `netdata-updater.sh` to `cron.daily`, so your Netdata installation will be updated daily (you will get a message from cron only if the update fails).
- For QA purposes, this installation method lets us know if it succeed or failed.

The `kickstart.sh` script passes all its parameters to `netdata-installer.sh`, so you can add more parameters to change the installation directory, enable/disable plugins, etc (check below).

For automated installs, append a space + `--dont-wait` to the command line. You can also append `--dont-start-it` to prevent the installer from starting Netdata. Example:

```bash
  bash <(curl -Ss https://my-netdata.io/kickstart.sh) --dont-wait --dont-start-it
```

If you don't want to receive automatic updates, add `--no-updates` when executing `kickstart.sh` script.

</details>&nbsp;<br/>

Once Netdata is installed, see [Getting Started](../../docs/GettingStarted.md).

---

## Linux 64bit pre-built static binary

You can install a pre-compiled static binary of Netdata on any Intel/AMD 64bit Linux system
(even those that don't have a package manager, like CoreOS, CirrOS, busybox systems, etc).
You can also use these packages on systems with broken or unsupported package managers.

To install Netdata with a binary package on any Linux distro, any kernel version - for **Intel/AMD 64bit** hosts, run the following:

```bash

  bash <(curl -Ss https://my-netdata.io/kickstart-static64.sh)

```

*(do not `sudo` this command, it will do it by itself as needed; if the target system does not have `bash` installed, see below for instructions to run it without `bash`)*

![](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.requests_per_url&options=unaligned&dimensions=kickstart64&group=sum&after=-3600&label=last+hour&units=installations&value_color=orange&precision=0) ![](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.requests_per_url&options=unaligned&dimensions=kickstart64&group=sum&after=-86400&label=today&units=installations&precision=0)

> The static builds install Netdata at **`/opt/netdata`**

<details markdown="1"><summary>Click here for more information and advanced use of this command.</summary>

&nbsp;<br/>
Verify the integrity of the script with this:

```bash
[ "9a8fec4af9aba11614381f9cd187a68b" = "$(curl -Ss https://my-netdata.io/kickstart-static64.sh | md5sum | cut -d ' ' -f 1)" ] && echo "OK, VALID" || echo "FAILED, INVALID"
```

*It should print `OK, VALID` if the script is the one we ship.*

For automated installs, append a space + `--dont-wait` to the command line. You can also append `--dont-start-it` to prevent the installer from starting Netdata.

Example:

```bash

  bash <(curl -Ss https://my-netdata.io/kickstart-static64.sh) --dont-wait --dont-start-it

```

If your shell fails to handle the above one liner, do this:

```bash
# download the script with curl
curl https://my-netdata.io/kickstart-static64.sh >/tmp/kickstart-static64.sh

# or, download the script with wget
wget -O /tmp/kickstart-static64.sh https://my-netdata.io/kickstart-static64.sh

# run the downloaded script (any sh is fine, no need for bash)
sh /tmp/kickstart-static64.sh
```

- The static binary files are kept in repo [binary-packages](https://github.com/netdata/binary-packages). You can download any of the `.run` files, and run it. These files are self-extracting shell scripts built with [makeself](https://github.com/megastep/makeself).
- The target system does **not** need to have bash installed.
- The same files can be used for updates too.
- For QA purposes, this installation method lets us know if it succeed or failed.

</details>&nbsp;<br/>

Once Netdata is installed, see [Getting Started](../../docs/GettingStarted.md).

---

## Run Netdata in a Docker container

You can [Install Netdata with Docker](../docker/#install-netdata-with-docker).

---

## Install Netdata on Linux manually

To install the latest git version of Netdata, please follow these 2 steps:

1. [Prepare your system](#prepare-your-system)

   Install the required packages on your system.

2. [Install Netdata](#install-netdata)

   Download and install Netdata. You can also update it the same way.

---

### Prepare your system

Try our experimental automatic requirements installer (no need to be root). This will try to find the packages that should be installed on your system to build and run Netdata. It supports most major Linux distributions released after 2010:

- **Alpine** Linux and its derivatives (you have to install `bash` yourself, before using the installer)
- **Arch** Linux and its derivatives
- **Gentoo** Linux and its derivatives
- **Debian** Linux and its derivatives (including **Ubuntu**, **Mint**)
- **Fedora** and its derivatives (including **Red Hat Enterprise Linux**, **CentOS**, **Amazon Machine Image**)
- **SuSe** Linux and its derivatives (including **openSuSe**)
- **SLE12** Must have your system registered with Suse Customer Center or have the DVD. See [#1162](https://github.com/netdata/netdata/issues/1162)

Install the packages for having a **basic Netdata installation** (system monitoring and many applications, without  `mysql` / `mariadb`, `postgres`, `named`, hardware sensors and `SNMP`):

```sh
curl -Ss 'https://raw.githubusercontent.com/netdata/netdata-demo-site/master/install-required-packages.sh' >/tmp/kickstart.sh && bash /tmp/kickstart.sh -i netdata
```

Install all the required packages for **monitoring everything Netdata can monitor**:

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

Once Netdata is compiled, to run it the following packages are required (already installed using the above commands):

package|description
:-----:|-----------
`libuuid`|part of `util-linux` for GUIDs management
`zlib`|gzip compression for the internal Netdata web server

*Netdata will fail to start without the above.*

Netdata plugins and various aspects of Netdata can be enabled or benefit when these are installed (they are optional):

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

*Netdata will greatly benefit if you have the above packages installed, but it will still work without them.*

---

### Install Netdata

Do this to install and run Netdata:

```sh

# download it - the directory 'netdata' will be created
git clone https://github.com/netdata/netdata.git --depth=100
cd netdata

# run script with root privileges to build, install, start Netdata
./netdata-installer.sh

```

* If you don't want to run it straight-away, add `--dont-start-it` option.

* If you don't want to install it on the default directories, you can run the installer like this: `./netdata-installer.sh --install /opt`. This one will install Netdata in `/opt/netdata`.

* If your server does not have access to the internet and you have manually put the installation directory on your server, you will need to pass the option `--disable-go` to the installer. The option will prevent the installer from attempting to download and install `go.d.plugin`. 

Once the installer completes, the file `/etc/netdata/netdata.conf` will be created (if you changed the installation directory, the configuration will appear in that directory too).

You can edit this file to set options. One common option to tweak is `history`, which controls the size of the memory database Netdata will use. By default is `3600` seconds (an hour of data at the charts) which makes Netdata use about 10-15MB of RAM (depending on the number of charts detected on your system). Check **[[Memory Requirements]]**.

To apply the changes you made, you have to restart Netdata.

---

## Other Systems



##### FreeBSD

You can install Netdata from ports or packages collection.

This is how to install the latest Netdata version from sources on FreeBSD:

```sh
# install required packages
pkg install bash e2fsprogs-libuuid git curl autoconf automake pkgconf pidof

# download Netdata
git clone https://github.com/netdata/netdata.git --depth=100

# install Netdata in /opt/netdata
cd netdata
./netdata-installer.sh --install /opt
```

##### pfSense
To install Netdata on pfSense run the following commands (within a shell or under Diagnostics/Command Prompt within the pfSense web interface).

Change platform (i386/amd64, etc) and FreeBSD versions (10/11, etc) according to your environment and change Netdata version (1.10.0 in example) according to latest version present within the FreeSBD repository:-

Note first three packages are downloaded from the pfSense repository for maintaining compatibility with pfSense, Netdata is downloaded from the FreeBSD repository.
```
pkg install pkgconf
pkg install bash
pkg install e2fsprogs-libuuid
pkg add http://pkg.freebsd.org/FreeBSD:11:amd64/latest/All/netdata-1.11.0.txz
```
To start Netdata manually run `service netdata onestart`

To start Netdata automatically at each boot add `service netdata start` as a Shellcmd within the pfSense web interface (under **Services/Shellcmd**, which you need to install beforehand under **System/Package Manager/Available Packages**).
Shellcmd Type should be set to `Shellcmd`.
![](https://user-images.githubusercontent.com/36808164/36930790-4db3aa84-1f0d-11e8-8752-cdc08bb7207c.png)
Alternatively more information can be found in https://doc.pfsense.org/index.php/Installing_FreeBSD_Packages, for achieving the same via the command line and scripts.

If you experience an issue with `/usr/bin/install` absense on pfSense 2.3 or earlier, update pfSense or use workaround from [https://redmine.pfsense.org/issues/6643](https://redmine.pfsense.org/issues/6643)

##### FreeNAS
On FreeNAS-Corral-RELEASE (>=10.0.3), Netdata is pre-installed.

To use Netdata, the service will need to be enabled and started from the FreeNAS **[CLI](https://github.com/freenas/cli)**.

To enable the Netdata service:
```
service netdata config set enable=true
```

To start the netdata service:
```
service netdata start
```

##### macOS

Netdata on macOS still has limited charts, but external plugins do work.

You can either install Netdata with [Homebrew](https://brew.sh/)

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

# download Netdata
git clone https://github.com/netdata/netdata.git --depth=100

# install Netdata in /usr/local/netdata
cd netdata
sudo ./netdata-installer.sh --install /usr/local
```

The installer will also install a startup plist to start Netdata when your Mac boots.

##### Alpine 3.x

Execute these commands to install Netdata in Alpine Linux 3.x:

```
# install required packages
apk add alpine-sdk bash curl zlib-dev util-linux-dev libmnl-dev gcc make git autoconf automake pkgconfig python logrotate

# if you plan to run node.js Netdata plugins
apk add nodejs

# download Netdata - the directory 'netdata' will be created
git clone https://github.com/netdata/netdata.git --depth=100
cd netdata


# build it, install it, start it
./netdata-installer.sh


# make Netdata start at boot
echo -e "#!/usr/bin/env bash\n/usr/sbin/netdata" >/etc/local.d/netdata.start
chmod 755 /etc/local.d/netdata.start

# make Netdata stop at shutdown
echo -e "#!/usr/bin/env bash\nkillall netdata" >/etc/local.d/netdata.stop
chmod 755 /etc/local.d/netdata.stop

# enable the local service to start automatically
rc-update add local
```

##### Synology

The documentation previously recommended installing the Debian Chroot package from the Synology community package sources and then running Netdata from within the chroot. This does not work, as the chroot environment does not have access to `/proc`, and therefore exposes very few metrics to Netdata. Additionally, [this issue](https://github.com/SynoCommunity/spksrc/issues/2758), still open as of 2018/06/24, indicates that the Debian Chroot package is not suitable for DSM versions greater than version 5 and may corrupt system libraries and render the NAS unable to boot.

The good news is that the 64-bit static installer works fine if your NAS is one that uses the amd64 architecture. It will install the content into `/opt/netdata`, making future removal safe and simple.

When Netdata is first installed, it will run as _root_. This may or may not be acceptable for you, and since other installations run it as the _netdata_ user, you might wish to do the same. This requires some extra work:

1. Creat a group `netdata` via the Synology group interface. Give it no access to anything.
2. Create a user `netdata` via the Synology user interface. Give it no access to anything and a random password. Assign the user to the `netdata` group. Netdata will chuid to this user when running.
3. Change ownership of the following directories, as defined in [Netdata Security](../../docs/netdata-security.md#security-design):

```
$ chown -R root:netdata /opt/netdata/usr/share/netdata
$ chown -R netdata:netdata /opt/netdata/var/lib/netdata /opt/netdata/var/cache/netdata
$ chown -R netdata:root /opt/netdata/var/log/netdata
```

Additionally, as of 2018/06/24, the Netdata installer doesn't recognize DSM as an operating system, so no init script is installed. You'll have to do this manually:

1. Add [this file](https://gist.github.com/oskapt/055d474d7bfef32c49469c1b53e8225f) as `/etc/rc.netdata`. Make it executable with `chmod 0755 /etc/rc.netdata`.
2. Edit `/etc/rc.local` and add a line calling `/etc/rc.netdata` to have it start on boot:

```
# Netdata startup
[ -x /etc/rc.netdata ] && /etc/rc.netdata start
```

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Finstaller%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)]()
