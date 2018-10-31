# Install netdata on Linux manually

To install the latest git version of netdata, please follow these 2 steps:

1. [Prepare your system]

   Install the required packages on your system.

2. [Install netdata]

   Download and install netdata. You can also update it the same way.

---

## Prepare your system

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
curl -Ss 'https://raw.githubusercontent.com/firehol/netdata-demo-site/master/install-required-packages.sh' >/tmp/kickstart.sh && bash /tmp/kickstart.sh -i netdata
```

Install all the required packages for **monitoring everything netdata can monitor**:

```sh
curl -Ss 'https://raw.githubusercontent.com/firehol/netdata-demo-site/master/install-required-packages.sh' >/tmp/kickstart.sh && bash /tmp/kickstart.sh -i netdata-all
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

## Install netdata

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
