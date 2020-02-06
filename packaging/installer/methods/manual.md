# Install Netdata on Linux manually

To install the latest git version of Netdata, please follow these 2 steps:

1.  [Prepare your system](#prepare-your-system)

    Install the required packages on your system.

2.  [Install Netdata](#install-netdata)

    Download and install Netdata. You can also update it the same way.

## Prepare your system

Try our experimental automatic requirements installer (no need to be root). This will try to find the packages that
should be installed on your system to build and run Netdata. It supports most major Linux distributions released after
2010:

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
    -   CentOS 8 / RHEL 8 requires a bit of extra work. See the dedicated section below.

-   **SuSe** Linux and its derivatives (including **openSuSe**)

-   **SLE12** Must have your system registered with Suse Customer Center or have the DVD. See
    [#1162](https://github.com/netdata/netdata/issues/1162)

Install the packages for having a **basic Netdata installation** (system monitoring and many applications, without  `mysql` / `mariadb`, `postgres`, `named`, hardware sensors and `SNMP`):

```sh
curl -Ss 'https://raw.githubusercontent.com/netdata/netdata-demo-site/master/install-required-packages.sh' >/tmp/install-required-packages.sh && bash /tmp/install-required-packages.sh -i netdata
```

Install all the required packages for **monitoring everything Netdata can monitor**:

```sh
curl -Ss 'https://raw.githubusercontent.com/netdata/netdata-demo-site/master/install-required-packages.sh' >/tmp/install-required-packages.sh && bash /tmp/install-required-packages.sh -i netdata-all
```

If the above do not work for you, please [open a github
issue](https://github.com/netdata/netdata/issues/new?title=packages%20installer%20failed&labels=installation%20help&body=The%20experimental%20packages%20installer%20failed.%0A%0AThis%20is%20what%20it%20says:%0A%0A%60%60%60txt%0A%0Aplease%20paste%20your%20screen%20here%0A%0A%60%60%60)
with a copy of the message you get on screen. We are trying to make it work everywhere (this is also why the script
[reports back](https://github.com/netdata/netdata/issues/2054) success or failure for all its runs).

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

### CentOS / RHEL 8.x

For CentOS / RHEL 8.x a lot of development packages have moved out into their
own separate repositories. Some other dependeicies are either missing completely
or have to be sourced by 3rd-parties.

CentOS 8.x:

- Enable the PowerTools repo
- Enable the EPEL repo
- Enable the Extra repo from [extra.getpagespeed.com](https://extras.getpagespeed.com/release-el8-latest.rpm)

And install the minimum required dependencies:

```sh
# Enable config-manager
yum install -y 'dnf-command(config-manager)'

# Enable PowerTools
yum config-manager --set-enabled PowerTools

# Enable EPEL
yum install -y epel-release

# Install Repo for libuv-devl (NEW)
yum install -y https://extras.getpagespeed.com/release-el8-latest.rpm

# Install Devel Packages
yum install autoconf automake curl gcc git libmnl-devel libuuid-devel openssl-devel libuv-devel lz4-devel make nc pkgconfig python3 zlib-devel

# Install Judy-Devel directly
yum install -y http://mirror.centos.org/centos/8/PowerTools/x86_64/os/Packages/Judy-devel-1.0.5-18.module_el8.1.0+217+4d875839.x86_64.rpm
```

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

## What's next?

When you finish installing Netdata, be sure to visit our [step-by-step tutorial](../../../docs/step-by-step/step-00.md)
for a fully-guided tour into Netdata's capabilities and how to configure it according to your needs.

Or, if you're a monitoring and system administration pro, skip ahead to our [getting started
guide](../../../docs/getting-started.md) for a quick overview.
