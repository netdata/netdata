# Manual installation

To install the latest version of Netdata from our [GitHub repository](https://github.com/netdata/netdata), please follow these two steps:

1. [Prepare your system](#prepare-your-system): Install the required packages on your system.
2. [Install Netdata](#install-netdata): Download and install Netdata. You can also update it the same way.


## Prepare your system

To prepare your system, you can either use our [automatic requirements installer](#prepare-your-system-with-our-automatic-requirements-installer) or install required packages [manually](#prepare-your-system-manually).


### Prepare your system with our automatic requirements installer

Use our experimental automatic requirements installer to find and install the packages that should be present on your system to build and run Netdata. This installer script supports most major Linux distributions released after 2010, and there's no need to use it as the `root` user:

- **Alpine** Linux and its derivatives
    - You have to install `bash` yourself before using the installer.
- **Arch** Linux and its derivatives
    - You need arch/aur for package Judy.
- **Gentoo** Linux and its derivatives
- **Debian** Linux and its derivatives (including **Ubuntu** and **Mint**)
- **Redhat Enterprise Linux** and its derivatives (including **Fedora**, **CentOS**, **Amazon Machine Image**)
    - Please note that for RHEL/CentOS you need the [EPEL repository](http://www.tecmint.com/how-to-enable-epel-repository-for-rhel-centos-6-5/). 
    - In addition, RHEL/CentOS version 6 also needs [OKay](https://okay.com.mx/blog-news/rpm-repositories-for-centos-6-and-7.html) for package `libuv` version 1.
- **SuSe** Linux and its derivatives (including **openSuSe**)
- **SLE12** Must have your system registered with Suse Customer Center or have the DVD. See [#1162](https://github.com/netdata/netdata/issues/1162)

For a **basic Netdata installation** (system monitoring and many applications, without  `mysql` / `mariadb`, `postgres`, `named`, hardware sensors and `SNMP`), install the following packages:

```sh
curl -Ss 'https://raw.githubusercontent.com/netdata/netdata-demo-site/master/install-required-packages.sh' >/tmp/kickstart.sh && bash /tmp/kickstart.sh -i netdata
```

If you would rather be able to **monitor all metrics native to Netdata**, use the installer script this way:

```sh
curl -Ss 'https://raw.githubusercontent.com/netdata/netdata-demo-site/master/install-required-packages.sh' >/tmp/kickstart.sh && bash /tmp/kickstart.sh -i netdata-all
```

Once finished, you can move on to [install Netdata](#install-netdata).

<details markdown="1"><summary>Troubleshooting the installer script</summary>

If the above command(s) do not work for you, please [open an issue on GitHub](https://github.com/netdata/netdata/issues/new?title=packages%20installer%20failed&labels=installation%20help&body=The%20experimental%20packages%20installer%20failed.%0A%0AThis%20is%20what%20it%20says:%0A%0A%60%60%60txt%0A%0Aplease%20paste%20your%20screen%20here%0A%0A%60%60%60) with a copy of the message you get on screen. 

We're working hard to make this script work everywhere, and so your issues are invaluable to us. The script [reports back](https://github.com/netdata/netdata/issues/2054) success or failure for all its runs so we can use the information to improve it.

</details>

### Prepare your system manually

Here's how to install the required packages directly using your distribution's package manager instead of the automatic script:

```bash
# Debian / Ubuntu
apt-get install zlib1g-dev uuid-dev libuv1-dev liblz4-dev libjudy-dev libssl-dev libmnl-dev gcc make git autoconf autoconf-archive autogen automake pkg-config curl python

# Fedora
dnf install zlib-devel libuuid-devel libuv-devel lz4-devel Judy-devel openssl-devel libmnl-devel gcc make git autoconf autoconf-archive autogen automake pkgconfig curl findutils python

# CentOS / Red Hat Enterprise Linux
yum install autoconf automake curl gcc git libmnl-devel libuuid-devel openssl-devel libuv-devel lz4-devel Judy-devel make nc pkgconfig python zlib-devel

# openSUSE
zypper install zlib-devel libuuid-devel libuv-devel liblz4-devel judy-devel libopenssl-devel libmnl-devel gcc make git autoconf autoconf-archive autogen automake pkgconfig curl findutils python
```

<details markdown="1"><summary>What are these packages for?</summary>

**Required**: Netdata will **fail to start** without the following:

package|description
:-----:|-----------
`libuuid`|part of `util-linux` for GUIDs management
`zlib`|gzip compression for the internal Netdata web server

**Required for DB engine**: While optional to run Netdata, these packages are required for the [DB engine](../../database/engine/).

|package|description|
|:-----:|-----------|
|`libuv`|multi-platform support library with a focus on asynchronous I/O|
|`liblz4`|Extremely Fast Compression algorithm|
|`Judy`|General purpose dynamic array|
|`openssl`|Cryptography and SSL/TLS Toolkit|

**Optional for collection and plugins**: These packages enable or further enhance Netdata's metrics collection capabilities, and greatly enhance its effectiveness, but are ultimately optional:

package|description
:-----:|-----------
`bash`|for shell plugins and **alarm notifications**
`curl`|for shell plugins and **alarm notifications**
`iproute` or `iproute2`|for monitoring **Linux traffic QoS**<br/>use `iproute2` if `iproute` reports as not available or obsolete
`python`|for most of the external plugins
`python-yaml`|used for monitoring **beanstalkd**
`python-beanstalkc`|used for monitoring **beanstalkd**
`python-dnspython`|used for monitoring DNS query time
`python-ipaddress`|used for monitoring **DHCPd**<br/>Netdata requires this package if the system has python v2. python v3 has this functionality embedded
`python-mysqldb`<br/>or<br/>`python-pymysql`|used for monitoring **mysql** or **mariadb** databases<br/>`python-mysqldb` is a lot faster and thus preferred
`python-psycopg2`|used for monitoring **postgresql** databases
`python-pymongo`|used for monitoring **mongodb** databases
`nodejs`|used for `node.js` plugins for monitoring **named** and **SNMP** devices
`lm-sensors`|for monitoring **hardware sensors**
`libmnl`|for collecting netfilter metrics
`netcat`|for shell plugins to collect metrics from remote systems

</details>


## Install Netdata

Do this to install and run Netdata:

```sh

# Download it to the `netdata` directory
git clone https://github.com/netdata/netdata.git --depth=100
cd netdata

# Run script with root privileges to build, install, and start Netdata
./netdata-installer.sh

```

* If you don't want to run it straight-away, add `--dont-start-it` option.

* You can also append `--stable-channel` to fetch and install only the official releases from GitHub, instead of the nightly builds.

* If you don't want to install it on the default directories, you can run the installer like this: `./netdata-installer.sh --install /opt`. This installs Netdata in `/opt/netdata`.

* If your server does not have access to the internet and you have manually put the installation directory on your server, you need to pass the option `--disable-go` to the installer. The option prevents the installer from attempting to download and install `go.d.plugin`. 

As the installer finishes, it creates the file `/etc/netdata/netdata.conf`. If you changed the installation directory, that configuration file appears wherever you specified.

You can edit this file to set options. One common option to tweak is `history`, which controls the size of the memory database Netdata uses. By default is `3600` seconds (an hour of data at the charts) which makes Netdata use about 10-15MB of RAM (depending on the number of charts detected on your system). Check out the [memory requirements](../../database/#database) to understand how much memory Netdata will use at different `history` values.

Restart Netdata to apply any of the changes you've made.