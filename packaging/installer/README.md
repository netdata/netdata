# Installation

Netdata is a **monitoring agent**. It is designed to be installed and run on all your systems: **physical** and **virtual** servers, **containers**, even **IoT**.

The best way to install Netdata is directly from source. Our **automatic installer** will install any required system packages and compile Netdata directly on your systems.

!!! warning
    You can find Netdata packages distributed by third parties. In many cases, these packages are either too old or broken. So, the suggested ways to install Netdata are the ones in this page.

1.  [Automatic one line installation](#one-line-installation), easy installation from source, **this is the default**
2.  [Install pre-built static binary on any 64bit Linux](#linux-64bit-pre-built-static-binary)
3.  [Run Netdata in a docker container](#run-netdata-in-a-docker-container)
4.  [Manual installation, step by step](#install-netdata-on-linux-manually)
5.  [Install on FreeBSD](#freebsd)
6.  [Install on pfSense](#pfsense)
7.  [Enable on FreeNAS Corral](#freenas)
8.  [Install on macOS (OS X)](#macos)
9.  [Install on a Kubernetes cluster](https://github.com/netdata/helmchart#netdata-helm-chart-for-kubernetes-deployments)
10. [Install using binary packages](#binary-packages)

See also the list of Netdata [package maintainers](../maintainers) for ASUSTOR NAS, OpenWRT, ReadyNAS, etc.

Note: From Netdata v1.12 and above, anonymous usage information is collected by default and sent to Google Analytics. To read more about the information collected and how to opt-out, check the [anonymous statistics page](../../docs/anonymous-statistics.md).

---

## One-line installation

![](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.requests_per_url&options=unaligned&dimensions=kickstart&group=sum&after=-3600&label=last+hour&units=installations&value_color=orange&precision=0) ![](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.requests_per_url&options=unaligned&dimensions=kickstart&group=sum&after=-86400&label=today&units=installations&precision=0)

This method is **fully automatic on all Linux distributions**. FreeBSD and MacOS systems need some preparations before installing Netdata for the first time. Check the [FreeBSD](#freebsd) and the [MacOS](#macos) sections for more information.

To install Netdata from source, and keep it up to date with our **nightly releases** automatically, run the following:

```bash
bash <(curl -Ss https://my-netdata.io/kickstart.sh)
```

!!! note
    Do not use `sudo` for the one-line installer—it will escalate privileges itself if needed.


To learn more about the pros and cons of using *nightly* vs. *stable* releases, see our [notice about the two options](#nightly-vs-stable-releases).

<details markdown="1"><summary>Click here for more information and advanced use of the one-line installation script.</summary>

Verify the integrity of the script with this:

```bash
[ "0ae8dd3c4c9b976c4342c9fc09d9afae" = "$(curl -Ss https://my-netdata.io/kickstart.sh | md5sum | cut -d ' ' -f 1)" ] && echo "OK, VALID" || echo "FAILED, INVALID"
```

_It should print `OK, VALID` if the script is the one we ship._

The `kickstart.sh` script:

-   detects the Linux distro and **installs the required system packages** for building Netdata (will ask for confirmation)
-   downloads the latest Netdata source tree to `/usr/src/netdata.git`.
-   installs Netdata by running `./netdata-installer.sh` from the source tree.
-   installs `netdata-updater.sh` to `cron.daily`, so your Netdata installation will be updated daily (you will get a message from cron only if the update fails).
-   For QA purposes, this installation method lets us know if it succeed or failed.

The `kickstart.sh` script passes all its parameters to `netdata-installer.sh`, so you can add more parameters to customize your installation. Here are a few important parameters:

-   `--dont-wait`: Enable automated installs by not prompting for permission to install any required packages.
-   `--dont-start-it`: Prevent the installer from starting Netdata automatically.
-   `--stable-channel`: Automatically update only on the release of new major versions.
-   `--nightly-channel`: Automatically update on every new nightly build.
-   `--no-updates`: Prevent automatic updates of any kind.
-   `--local-files`: Used for offline installations. Pass four file paths: the Netdata tarball, the checksum file, the go.d plugin tarball, and the go.d plugin config tarball, to force kickstart run the process using those files.

Example using all the above parameters:

```bash
bash <(curl -Ss https://my-netdata.io/kickstart.sh) --dont-wait --dont-start-it --no-updates --stable-channel --local-files /tmp/my-selfdownloaded-tarball.tar.gz /tmp/checksums.txt /tmp/manually.downloaded.go.d.binary.tar.gz /tmp/manually.downloaded.go.d.config.tar.gz
```
Note: `--stable-channel` and `--local-files` overlap, if you use the tarball override the stable channel option is not effective
</details>

Now that Netdata is installed, be sure to visit our [getting started guide](../../docs/getting-started.md) for a quick
overview of configuring Netdata, enabling plugins, and controlling Netdata's daemon. Or, get the full guided tour of
Netdata's capabilities with our [step-by-step tutorial](../../docs/step-by-step/step-00.md)!

---

## Linux 64bit pre-built static binary

![](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.requests_per_url&options=unaligned&dimensions=kickstart64&group=sum&after=-3600&label=last+hour&units=installations&value_color=orange&precision=0) ![](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.requests_per_url&options=unaligned&dimensions=kickstart64&group=sum&after=-86400&label=today&units=installations&precision=0)

You can install a pre-compiled static binary of Netdata on any Intel/AMD 64bit Linux system (even those that don't have a package manager, like CoreOS, CirrOS, busybox systems, etc). You can also use these packages on systems with broken or unsupported package managers.

To install Netdata from a binary package on any Linux distro and any kernel version on **Intel/AMD 64bit** systems, and keep it up to date with our **nightly releases** automatically, run the following:

```bash
bash <(curl -Ss https://my-netdata.io/kickstart-static64.sh)
```

!!! note
    Do not use `sudo` for this installer—it will escalate privileges itself if needed.

To learn more about the pros and cons of using *nightly* vs. *stable* releases, see our [notice about the two options](README.md#nightly-vs-stable-releases).

If your system does not have `bash` installed, open the `More information and advanced uses of the kickstart-static64.sh script` dropdown for instructions to run the installer without `bash`.

This script installs Netdata at `/opt/netdata`.

<details markdown="1"><summary>Click here for more information and advanced use of this command.</summary>

Verify the integrity of the script with this:

```bash
[ "23e0f38dfb9d517be16393c3ed1f88bd" = "$(curl -Ss https://my-netdata.io/kickstart-static64.sh | md5sum | cut -d ' ' -f 1)" ] && echo "OK, VALID" || echo "FAILED, INVALID"
```

*It should print `OK, VALID` if the script is the one we ship.*

The `kickstart-static64.sh` script passes all its parameters to `netdata-installer.sh`, so you can add more parameters to customize your installation. Here are a few important parameters:

-   `--dont-wait`: Enable automated installs by not prompting for permission to install any required packages.
-   `--dont-start-it`: Prevent the installer from starting Netdata automatically.
-   `--stable-channel`: Automatically update only on the release of new major versions.
-   `--no-updates`: Prevent automatic updates of any kind.
-   `--local-files`: Used for offline installations. Pass two file paths, one for the tarball and one for the checksum file, to force kickstart run the process using those files.

Example using all the above parameters:

```sh
bash <(curl -Ss https://my-netdata.io/kickstart-static64.sh) --dont-wait --dont-start-it --no-updates --stable-channel --local-files /tmp/my-selfdownloaded-tarball.tar.gz /tmp/checksums.txt
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

-   The static binary files are kept in repo [binary-packages](https://github.com/netdata/binary-packages). You can download any of the `.run` files, and run it. These files are self-extracting shell scripts built with [makeself](https://github.com/megastep/makeself).
-   The target system does **not** need to have bash installed.
-   The same files can be used for updates too.
-   For QA purposes, this installation method lets us know if it succeed or failed.

</details>

Now that Netdata is installed, be sure to visit our [getting started guide](../../docs/getting-started.md) for a quick
overview of configuring Netdata, enabling plugins, and controlling Netdata's daemon. Or, get the full guided tour of
Netdata's capabilities with our [step-by-step tutorial](../../docs/step-by-step/step-00.md)!

---

## Run Netdata in a Docker container

You can [Install Netdata with Docker](../docker/#install-netdata-with-docker).

---

## Install Netdata on Linux manually

To install the latest git version of Netdata, please follow these 2 steps:

1.  [Prepare your system](#prepare-your-system)

    Install the required packages on your system.

2.  [Install Netdata](#install-netdata)

    Download and install Netdata. You can also update it the same way.

---

### Prepare your system

Try our experimental automatic requirements installer (no need to be root). This will try to find the packages that should be installed on your system to build and run Netdata. It supports most major Linux distributions released after 2010:

-   **Alpine** Linux and its derivatives
    -   You have to install `bash` yourself, before using the installer.

-   **Arch** Linux and its derivatives
    -   You need arch/aur for package Judy.

-   **Gentoo** Linux and its derivatives

-   **Debian** Linux and its derivatives (including **Ubuntu**, **Mint**)

-   **Redhat Enterprise Linux** and its derivatives (including **Fedora**, **CentOS**, **Amazon Machine Image**)
    -   Please note that for RHEL/CentOS you need
        [EPEL](http://www.tecmint.com/how-to-enable-epel-repository-for-rhel-centos-6-5/).
        In addition, RHEL/CentOS version 6 also need
        [OKay](https://okay.com.mx/blog-news/rpm-repositories-for-centos-6-and-7.html) for package libuv version 1.

-   **SuSe** Linux and its derivatives (including **openSuSe**)

-   **SLE12** Must have your system registered with Suse Customer Center or have the DVD. See [#1162](https://github.com/netdata/netdata/issues/1162)

Install the packages for having a **basic Netdata installation** (system monitoring and many applications, without  `mysql` / `mariadb`, `postgres`, `named`, hardware sensors and `SNMP`):

```sh
curl -Ss 'https://raw.githubusercontent.com/netdata/netdata-demo-site/master/install-required-packages.sh' >/tmp/install-required-packages.sh && bash /tmp/install-required-packages.sh -i netdata
```

Install all the required packages for **monitoring everything Netdata can monitor**:

```sh
curl -Ss 'https://raw.githubusercontent.com/netdata/netdata-demo-site/master/install-required-packages.sh' >/tmp/install-required-packages.sh && bash /tmp/install-required-packages.sh -i netdata-all
```

If the above do not work for you, please [open a github issue](https://github.com/netdata/netdata/issues/new?title=packages%20installer%20failed&labels=installation%20help&body=The%20experimental%20packages%20installer%20failed.%0A%0AThis%20is%20what%20it%20says:%0A%0A%60%60%60txt%0A%0Aplease%20paste%20your%20screen%20here%0A%0A%60%60%60) with a copy of the message you get on screen. We are trying to make it work everywhere (this is also why the script [reports back](https://github.com/netdata/netdata/issues/2054) success or failure for all its runs).

---

This is how to do it by hand:

```sh
# Debian / Ubuntu
apt-get install zlib1g-dev uuid-dev libuv1-dev liblz4-dev libjudy-dev libssl-dev libmnl-dev gcc make git autoconf autoconf-archive autogen automake pkg-config curl python

# Fedora
dnf install zlib-devel libuuid-devel libuv-devel lz4-devel Judy-devel openssl-devel libmnl-devel gcc make git autoconf autoconf-archive autogen automake pkgconfig curl findutils python

# CentOS / Red Hat Enterprise Linux
yum install autoconf automake curl gcc git libmnl-devel libuuid-devel openssl-devel libuv-devel lz4-devel Judy-devel make nc pkgconfig python zlib-devel

# openSUSE
zypper install zlib-devel libuuid-devel libuv-devel liblz4-devel judy-devel libopenssl-devel libmnl-devel gcc make git autoconf autoconf-archive autogen automake pkgconfig curl findutils python
```

Once Netdata is compiled, to run it the following packages are required (already installed using the above commands):

| package   | description|
|:-----:|-----------|
| `libuuid` | part of `util-linux` for GUIDs management|
| `zlib`    | gzip compression for the internal Netdata web server|
| `libuv`   | Multi-platform support library with a focus on asynchronous I/O, version 1 or greater|

*Netdata will fail to start without the above.*

Netdata plugins and various aspects of Netdata can be enabled or benefit when these are installed (they are optional):

| package |description|
|:-----:|-----------|
| `bash`|for shell plugins and **alarm notifications**|
| `curl`|for shell plugins and **alarm notifications**|
| `iproute` or `iproute2`|for monitoring **Linux traffic QoS**<br/>use `iproute2` if `iproute` reports as not available or obsolete|
| `python`|for most of the external plugins|
| `python-yaml`|used for monitoring **beanstalkd**|
| `python-beanstalkc`|used for monitoring **beanstalkd**|
| `python-dnspython`|used for monitoring DNS query time|
| `python-ipaddress`|used for monitoring **DHCPd**<br/>this package is required only if the system has python v2. python v3 has this functionality embedded|
| `python-mysqldb`<br/>or<br/>`python-pymysql`|used for monitoring **mysql** or **mariadb** databases<br/>`python-mysqldb` is a lot faster and thus preferred|
| `python-psycopg2`|used for monitoring **postgresql** databases|
| `python-pymongo`|used for monitoring **mongodb** databases|
| `nodejs`|used for `node.js` plugins for monitoring **named** and **SNMP** devices|
| `lm-sensors`|for monitoring **hardware sensors**|
| `libmnl`|for collecting netfilter metrics|
| `netcat`|for shell plugins to collect metrics from remote systems|

*Netdata will greatly benefit if you have the above packages installed, but it will still work without them.*

Netdata DB engine can be enabled when these are installed (they are optional):

| package  | description|
|:-----:|-----------|
| `liblz4` | Extremely fast compression algorithm, version r129 or greater|
| `Judy`   | General purpose dynamic array|
| `openssl`| Cryptography and SSL/TLS toolkit|

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

-   If you don't want to run it straight-away, add `--dont-start-it` option.

-   You can also append `--stable-channel` to fetch and install only the official releases from GitHub, instead of the nightly builds.

-   If you don't want to install it on the default directories, you can run the installer like this: `./netdata-installer.sh --install /opt`. This one will install Netdata in `/opt/netdata`.

-   If your server does not have access to the internet and you have manually put the installation directory on your server, you will need to pass the option `--disable-go` to the installer. The option will prevent the installer from attempting to download and install `go.d.plugin`. 

Once the installer completes, the file `/etc/netdata/netdata.conf` will be created (if you changed the installation directory, the configuration will appear in that directory too).

You can edit this file to set options. One common option to tweak is `history`, which controls the size of the memory database Netdata will use. By default is `3600` seconds (an hour of data at the charts) which makes Netdata use about 10-15MB of RAM (depending on the number of charts detected on your system). Check **\[[Memory Requirements]]**.

To apply the changes you made, you have to restart Netdata.

---

### Binary Packages

![](https://raw.githubusercontent.com/netdata/netdata/master/web/gui/images/packaging-beta-tag.svg?sanitize=true)

We provide our own flavour of binary packages for the most common operating systems that comply with .RPM and .DEB packaging formats.

We have currently released packages following the .RPM format with version [1.16.0](https://github.com/netdata/netdata/releases/tag/v1.16.0).
We have planned to release packages following the .DEB format with version [1.17.0](https://github.com/netdata/netdata/releases/tag/v1.17.0).
Early adopters may experiment with our .DEB formatted packages using our nightly releases. Our current packaging infrastructure provider is [Package Cloud](https://packagecloud.io).

Netdata is committed to support installation of our solution to all operating systems. This is a constant battle for Netdata, as we strive to automate and make things easier for our users. For the operating system support matrix, please visit our [distributions](../../packaging/DISTRIBUTIONS.md) support page.

We provide two separate repositories, one for our stable releases and one for our nightly releases.

1.  Stable releases: Our stable production releases are hosted in [netdata/netdata](https://packagecloud.io/netdata/netdata) repository of package cloud
2.  Nightly releases: Our latest releases are hosted in [netdata/netdata-edge](https://packagecloud.io/netdata/netdata-edge) repository of package cloud

Visit the repository pages and follow the quick set-up instructions to get started.

---

## Other Systems

##### FreeBSD

You can install Netdata from ports or packages collection.

This is how to install the latest Netdata version from sources on FreeBSD:

```sh
# install required packages
pkg install bash e2fsprogs-libuuid git curl autoconf automake pkgconf pidof Judy liblz4 libuv json-c

# download Netdata
git clone https://github.com/netdata/netdata.git --depth=100

# install Netdata in /opt/netdata
cd netdata
./netdata-installer.sh --install /opt
```

##### pfSense

To install Netdata on pfSense, run the following commands (within a shell or under the **Diagnostics/Command** prompt within the pfSense web interface).

Note that the first four packages are downloaded from the pfSense repository for maintaining compatibility with pfSense, Netdata, Judy and Python are downloaded from the FreeBSD repository.

```sh
pkg install pkgconf
pkg install bash
pkg install e2fsprogs-libuuid
pkg install libuv
pkg add http://pkg.freebsd.org/FreeBSD:11:amd64/latest/All/Judy-1.0.5_2.txz
pkg add http://pkg.freebsd.org/FreeBSD:11:amd64/latest/All/python36-3.6.9.txz
ln -s /usr/local/lib/libjson-c.so /usr/local/lib/libjson-c.so.4
pkg add http://pkg.freebsd.org/FreeBSD:11:amd64/latest/All/netdata-1.17.1.txz
```
**Note:** If you receive a ` Not Found` error during the last two commands above, you will either need to manually look in the [repo folder](http://pkg.freebsd.org/FreeBSD:11:amd64/latest/All/) for the latest available package and use its URL instead, or you can try manually changing the netdata version in the URL to the latest version.  

You must edit `/usr/local/etc/netdata/netdata.conf` and change `bind to = 127.0.0.1` to `bind to = 0.0.0.0`.

To start Netdata manually, run `service netdata onestart`  

Visit the Netdata dashboard to confirm it's working: `http://<pfsenseIP>:19999`

To start Netdata automatically every boot, add `service netdata onestart` as a Shellcmd entry within the pfSense web interface under **Services/Shellcmd**. You'll need to install the Shellcmd package beforehand under **System/Package Manager/Available Packages**. The Shellcmd Type should be set to `Shellcmd`.  
![](https://i.imgur.com/wcKiPe1.png)
Alternatively more information can be found in <https://doc.pfsense.org/index.php/Installing_FreeBSD_Packages>, for achieving the same via the command line and scripts.

If you experience an issue with `/usr/bin/install` being absent in pfSense 2.3 or earlier, update pfSense or use a workaround from <https://redmine.pfsense.org/issues/6643>  

**Note:** In pfSense, the Netdata configuration files are located under `/usr/local/etc/netdata`

##### FreeNAS

On FreeNAS-Corral-RELEASE (>=10.0.3 and <11.3), Netdata is pre-installed.

To use Netdata, the service will need to be enabled and started from the FreeNAS **[CLI](https://github.com/freenas/cli)**.

To enable the Netdata service:

```sh
service netdata config set enable=true
```

To start the Netdata service:

```sh
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

```sh
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

When Netdata is first installed, it will run as *root*. This may or may not be acceptable for you, and since other installations run it as the *netdata* user, you might wish to do the same. This requires some extra work:

1.  Creat a group `netdata` via the Synology group interface. Give it no access to anything.
2.  Create a user `netdata` via the Synology user interface. Give it no access to anything and a random password. Assign the user to the `netdata` group. Netdata will chuid to this user when running.
3.  Change ownership of the following directories, as defined in [Netdata Security](../../docs/netdata-security.md#security-design):

```sh
chown -R root:netdata /opt/netdata/usr/share/netdata
chown -R netdata:netdata /opt/netdata/var/lib/netdata /opt/netdata/var/cache/netdata
chown -R netdata:root /opt/netdata/var/log/netdata
```

Additionally, as of 2018/06/24, the Netdata installer doesn't recognize DSM as an operating system, so no init script is installed. You'll have to do this manually:

1.  Add [this file](https://gist.github.com/oskapt/055d474d7bfef32c49469c1b53e8225f) as `/etc/rc.netdata`. Make it executable with `chmod 0755 /etc/rc.netdata`.
2.  Edit `/etc/rc.local` and add a line calling `/etc/rc.netdata` to have it start on boot:

```
# Netdata startup
[ -x /etc/rc.netdata ] && /etc/rc.netdata start
```

## Nightly vs. stable releases

The Netdata team maintains two releases of the Netdata agent: **nightly** and **stable**. By default, Netdata's installation scripts will give you **automatic, nightly** updates, as that is our recommended configuration.

**Nightly**: We create nightly builds every 24 hours. They contain fully-tested code that fixes bugs or security flaws, or introduces new features to Netdata. Every nightly release is a candidate for then becoming a stable release—when we're ready, we simply change the release tags on GitHub. That means nightly releases are stable and proven to function correctly in the vast majority of Netdata use cases. That's why nightly is the *best choice for most Netdata users*.

**Stable**: We create stable releases whenever we believe the code has reached a major milestone. Most often, stable releases correlate with the introduction of new, significant features. Stable releases might be a better choice for those who run Netdata in *mission-critical production systems*, as updates will come more infrequently, and only after the community helps fix any bugs that might have been introduced in previous releases.

**Pros of using nightly releases:**

-   Get the latest features and bugfixes as soon as they're available
-   Receive security-related fixes immediately
-   Use stable, fully-tested code that's always improving
-   Leverage the same Netdata experience our community is using

**Pros of using stable releases:**

-   Protect yourself from the rare instance when major bugs slip through our testing and negatively affect a Netdata installation
-   Retain more control over the Netdata version you use

## Offline installations

You can install Netdata on systems without internet access, but you need to take
a few extra steps to make it work.

By default, the `kickstart.sh` and `kickstart-static64.sh` download Netdata
assets, like the precompiled binary and a few dependencies, using the system's
internet connection, but you can also supply these files from the local filesystem.

First, download the required files. If you're using `kickstart.sh`, you need the
Netdata tarball, the checksums, the go.d plugin binary, and the go.d plugin
configuration. If you're using `kickstart-static64.sh`, you need only the
Netdata tarball and checksums.

Download the files you need to a system of yours that's connected to the
internet. You can use the commands below, or visit the [latest Netdata release
page](https://github.com/netdata/netdata/releases/latest) and [latest go.d
plugin release page](https://github.com/netdata/go.d.plugin/releases) to
download the required files manually.

#### kickstart.sh
```bash
cd /tmp

curl -s https://my-netdata.io/kickstart.sh > kickstart.sh

# Netdata tarball
curl -s https://api.github.com/repos/netdata/netdata/releases/latest | grep "browser_download_url.*tar.gz" | cut -d '"' -f 4 | wget -qi -

# Netdata checksums
curl -s https://api.github.com/repos/netdata/netdata/releases/latest | grep "browser_download_url.*txt" | cut -d '"' -f 4 | wget -qi -

# Netdata dependency handling script
curl -s https://raw.githubusercontent.com/netdata/netdata-demo-site/master/install-required-packages.sh | wget -qi -

# go.d plugin 
# For binaries for OS types and architectures not listed on [go.d releases](https://github.com/netdata/go.d.plugin/releases/latest), kindly open a github issue and we will do our best to serve your request
export OS=$(uname -s | tr '[:upper:]' '[:lower:]') ARCH=$(uname -m | sed -e 's/i386/386/g' -e 's/i686/386/g' -e 's/x86_64/amd64/g' -e 's/aarch64/arm64/g' -e 's/armv64/arm64/g' -e 's/armv6l/arm/g' -e 's/armv7l/arm/g' -e 's/armv5tel/arm/g') && curl -s https://api.github.com/repos/netdata/go.d.plugin/releases/latest | grep "browser_download_url.*${OS}-${ARCH}.tar.gz" | cut -d '"' -f 4 | wget -qi -

# go.d configuration 
curl -s https://api.github.com/repos/netdata/go.d.plugin/releases/latest | grep "browser_download_url.*config.tar.gz" | cut -d '"' -f 4 | wget -qi -
```

#### kickstart-static64.sh
```bash
cd /tmp

curl -s https://my-netdata.io/kickstart-static64.sh > kickstart-static64.sh

# Netdata static64 tarball
curl -s https://api.github.com/repos/netdata/netdata/releases/latest | grep "browser_download_url.*gz.run" | cut -d '"' -f 4 | wget -qi -

# Netdata checksums
curl -s https://api.github.com/repos/netdata/netdata/releases/latest | grep "browser_download_url.*txt" | cut -d '"' -f 4 | wget -qi -
```

Move downloaded files to the `/tmp` directory on the offline system in whichever way
your defined policy allows (if any).

Now you can run either the `kickstart.sh` or `kickstart-static64.sh` scripts
using the `--local-files` option. This option requires you to specify
the location and names of the files you just downloaded. 

!!! note When using `--local-files`, the `kickstart.sh` or
    `kickstart-static64.sh` scripts won't download any Netdata assets from the
    internet. But, you may still need a connection to install dependencies using
    your system's package manager. The scripts will warn you if your system
    doesn't have all the dependencies.

```bash
# kickstart.sh
bash kickstart.sh --local-files /tmp/netdata-version-number-here.tar.gz /tmp/sha256sums.txt /tmp/go.d-binary-filename.tar.gz /tmp/config.tar.gz /tmp/install-required-packages.sh

# kickstart-static64.sh
bash kickstart-static64.sh --local-files /tmp/netdata-version-number-here.gz.run /tmp/sha256sums.txt
```

Now that you're finished with your offline installation, you can move on to our [getting started
guide](../../docs/getting-started.md) for a quick overview of configuring Netdata, enabling plugins, and controlling
Netdata's daemon. Or, get the full guided tour of Netdata's capabilities with our [step-by-step
tutorial](../../docs/step-by-step/step-00.md)!

## Automatic updates

By default, Netdata's installation scripts enable automatic updates for both nightly and stable release channels.

If you would prefer to manually update your Netdata agent, you can disable automatic updates by using the `--no-updates` option when you install or update Netdata using the [one-line installation script](#one-line-installation).

```bash
# kickstart.sh
bash <(curl -Ss https://my-netdata.io/kickstart.sh) --no-updates

# kickstart-static64.sh
bash <(curl -Ss https://my-netdata.io/kickstart-static64.sh) --no-updates
```

With automatic updates disabled, you can choose exactly when and how you [update Netdata](UPDATE.md).

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Finstaller%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
