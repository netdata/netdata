<!--
title: "Install Netdata on Linux from a Git checkout"
description: "Use the Netdata Agent source code from GitHub, plus helper scripts to set up your system, to install Netdata without packages or binaries."
custom_edit_url: "https://github.com/netdata/netdata/edit/master/packaging/installer/methods/manual.md"
sidebar_label: "From a Git checkout"
learn_status: "Published"
learn_rel_path: "Installation/Installation methods"
sidebar_position: 30
-->

# Install Netdata on Linux from a Git checkout

To install the latest git version of Netdata, please follow these 2 steps:

1.  [Prepare your system](#prepare-your-system)

    Install the required packages on your system.

2.  [Install Netdata](#install-netdata)

    Download and install Netdata. You can also update it the same way.

## Prepare your system

Before you begin, make sure that your repo and the repo's submodules are clean from any previous builds and up to date.
Otherwise, [perform a cleanup](https://github.com/netdata/netdata/blob/master/packaging/installer/methods/manual.md#perform-a-cleanup-in-your-netdata-repo)

Use our automatic requirements installer (_no need to be `root`_), which attempts to find the packages that
should be installed on your system to build and run Netdata. It supports a large variety of major Linux distributions
and other operating systems and is regularly tested. You can find this tool [here](https://raw.githubusercontent.com/netdata/netdata/master/packaging/installer/install-required-packages.sh) or run it directly with `bash <(curl -sSL https://raw.githubusercontent.com/netdata/netdata/master/packaging/installer/install-required-packages.sh)`. Otherwise read on for how to get requires packages manually:

-   **Alpine** Linux and its derivatives
    -   You have to install `bash` yourself, before using the installer.

-   **Gentoo** Linux and its derivatives

-   **Debian** Linux and its derivatives (including **Ubuntu**, **Mint**)

-   **Red Hat Enterprise Linux** and its derivatives (including **Fedora**, **CentOS**, **Amazon Machine Image**)
    -   Please note that for RHEL/CentOS you need
        [EPEL](http://www.tecmint.com/how-to-enable-epel-repository-for-rhel-centos-6-5/).
        In addition, RHEL/CentOS version 6 also need
        [OKay](https://okay.com.mx/blog-news/rpm-repositories-for-centos-6-and-7.html) for package libuv version 1.
    -   CentOS 8 / RHEL 8 requires a bit of extra work. See the dedicated section below.

-   **SUSE** Linux and its derivatives (including **openSUSE**)

-   **SLE12** Must have your system registered with SUSE Customer Center or have the DVD. See
    [#1162](https://github.com/netdata/netdata/issues/1162)

Install the packages for having a **basic Netdata installation** (system monitoring and many applications, without  `mysql` / `mariadb`, `named`, hardware sensors and `SNMP`):

```sh
curl -Ss 'https://raw.githubusercontent.com/netdata/netdata/master/packaging/installer/install-required-packages.sh' >/tmp/install-required-packages.sh && bash /tmp/install-required-packages.sh -i netdata
```

Install all the required packages for **monitoring everything Netdata can monitor**:

```sh
curl -Ss 'https://raw.githubusercontent.com/netdata/netdata/master/packaging/installer/install-required-packages.sh' >/tmp/install-required-packages.sh && bash /tmp/install-required-packages.sh -i netdata-all
```

If the above do not work for you, please [open a github
issue](https://github.com/netdata/netdata/issues/new?title=packages%20installer%20failed&labels=installation%20help&body=The%20experimental%20packages%20installer%20failed.%0A%0AThis%20is%20what%20it%20says:%0A%0A%60%60%60txt%0A%0Aplease%20paste%20your%20screen%20here%0A%0A%60%60%60)
with a copy of the message you get on screen. We are trying to make it work everywhere (this is also why the script
[reports back](https://github.com/netdata/netdata/issues/2054) success or failure for all its runs).

---

This is how to do it by hand:

```sh
# Debian / Ubuntu
apt-get install zlib1g-dev uuid-dev libuv1-dev liblz4-dev libssl-dev libelf-dev libmnl-dev libprotobuf-dev protobuf-compiler gcc g++ make git autoconf autoconf-archive autogen automake pkg-config curl python cmake

# Fedora
dnf install zlib-devel libuuid-devel libuv-devel lz4-devel openssl-devel elfutils-libelf-devel libmnl-devel protobuf-devel protobuf-compiler gcc gcc-c++ make git autoconf autoconf-archive autogen automake pkgconfig curl findutils python cmake

# CentOS / Red Hat Enterprise Linux
yum install autoconf automake curl gcc gcc-c++ git libmnl-devel libuuid-devel openssl-devel libuv-devel lz4-devel elfutils-libelf-devel protobuf protobuf-devel protobuf-compiler make nc pkgconfig python zlib-devel cmake

# openSUSE
zypper install zlib-devel libuuid-devel libuv-devel liblz4-devel libopenssl-devel libelf-devel libmnl-devel protobuf-devel gcc gcc-c++ make git autoconf autoconf-archive autogen automake pkgconfig curl findutils python cmake
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
| `bash`|for shell plugins and **alert notifications**|
| `curl`|for shell plugins and **alert notifications**|
| `iproute` or `iproute2`|for monitoring **Linux traffic QoS**<br/>use `iproute2` if `iproute` reports as not available or obsolete|
| `python`|for most of the external plugins|
| `python-yaml`|used for monitoring **beanstalkd**|
| `python-beanstalkc`|used for monitoring **beanstalkd**|
| `python-mysqldb`<br/>or<br/>`python-pymysql`|used for monitoring **mysql** or **mariadb** databases<br/>`python-mysqldb` is a lot faster and thus preferred|
| `nodejs`|used for `node.js` plugins for monitoring **named** and **SNMP** devices|
| `lm-sensors`|for monitoring **hardware sensors**|
| `libelf`|for monitoring kernel-level metrics using eBPF|
| `libmnl`|for collecting netfilter metrics|
| `netcat`|for shell plugins to collect metrics from remote systems|

*Netdata will greatly benefit if you have the above packages installed, but it will still work without them.*

Netdata DB engine can be enabled when these are installed (they are optional):

| package  | description|
|:-----:|-----------|
| `liblz4` | Extremely fast compression algorithm, version r129 or greater|
| `openssl`| Cryptography and SSL/TLS toolkit|

*Netdata will greatly benefit if you have the above packages installed, but it will still work without them.*

Netdata Cloud support may require the following packages to be installed:

|  package  | description                                                                                                                          |
|:---------:|--------------------------------------------------------------------------------------------------------------------------------------|
|  `cmake`  | Needed at build time if you aren't using your distribution's version of libwebsockets or are building on a platform other than Linux |
| `openssl` | Needed to secure communications with the Netdata Cloud                                                                               |
| `protobuf`| Used for the new Cloud<->Agent binary protocol |

*Netdata will greatly benefit if you have the above packages installed, but it will still work without them.*

### CentOS / RHEL 6.x

On CentOS / RHEL 6.x, many of the dependencies for Netdata are only
available with versions older than what we need, so special setup is
required if manually installing packages.

CentOS 6.x:

- Enable the EPEL repo
- Enable the additional repo from [okay.network](https://okay.network/blog-news/rpm-repositories-for-centos-6-and-7.html)

And install the minimum required dependencies.

### CentOS / RHEL 8.x

For CentOS / RHEL 8.x a lot of development packages have moved out into their
own separate repositories. Some other dependencies are either missing completely
or have to be sourced by 3rd-parties.

CentOS 8.x:

- Enable the PowerTools repo
- Enable the EPEL repo
- Enable the Extra repo from [OKAY](https://okay.network/blog-news/rpm-repositories-for-centos-6-and-7.html)

And install the minimum required dependencies:

```sh
# Enable config-manager
yum install -y 'dnf-command(config-manager)'

# Enable PowerTools
yum config-manager --set-enabled powertools

# Enable EPEL
yum install -y epel-release

# Install Repo for libuv-devl (NEW)
yum install -y http://repo.okay.com.mx/centos/8/x86_64/release/okay-release-1-3.el8.noarch.rpm

# Install Devel Packages
yum install autoconf automake curl gcc git cmake libuuid-devel openssl-devel libuv-devel lz4-devel make nc pkgconfig python3 zlib-devel

```

## Install Netdata

Do this to install and run Netdata:

```sh
# download it - the directory 'netdata' will be created
git clone https://github.com/netdata/netdata.git --depth=100 --recursive
cd netdata

# run script with root privileges to build, install, start Netdata
./netdata-installer.sh
```

-   If you don't want to run it straight-away, add `--dont-start-it` option.

-   You can also append `--stable-channel` to fetch and install only the official releases from GitHub, instead of the nightly builds.

-   If you don't want to install it on the default directories, you can run the installer like this: `./netdata-installer.sh --install-prefix /opt`. This one will install Netdata in `/opt/netdata`.

-   If your server does not have access to the internet and you have manually put the installation directory on your server, you will need to pass the option `--disable-go` to the installer. The option will prevent the installer from attempting to download and install `go.d.plugin`. 

## Optional parameters to alter your installation

`netdata-installer.sh` accepts a few parameters to customize your installation:

-   `--dont-wait`: Enable automated installs by not prompting for permission to install any required packages.
-   `--dont-start-it`: Prevent the installer from starting Netdata automatically.
-   `--stable-channel`: Automatically update only on the release of new major versions.
-   `--nightly-channel`: Automatically update on every new nightly build.
-   `--disable-telemetry`: Opt-out of [anonymous statistics](https://github.com/netdata/netdata/blob/master/docs/netdata-agent/configuration/anonymous-telemetry-events.md) we use to make
    Netdata better.
-   `--no-updates`: Prevent automatic updates of any kind.
-   `--reinstall`: If an existing install is detected, reinstall instead of trying to update it. Note that this
    cannot be used to change installation types.
-   `--local-files`: Used for [offline installations](https://github.com/netdata/netdata/blob/master/packaging/installer/methods/offline.md). Pass four file paths: the Netdata
    tarball, the checksum file, the go.d plugin tarball, and the go.d plugin config tarball, to force kickstart run the
    process using those files. This option conflicts with the `--stable-channel` option. If you set this _and_
    `--stable-channel`, Netdata will use the local files.

### Connect node to Netdata Cloud during installation

Unlike the [`kickstart.sh`](https://github.com/netdata/netdata/blob/master/packaging/installer/methods/kickstart.md), the `netdata-installer.sh` script does
not allow you to automatically [connect](https://github.com/netdata/netdata/blob/master/src/claim/README.md) your node to Netdata Cloud immediately after installation.

See the [connect to cloud](https://github.com/netdata/netdata/blob/master/src/claim/README.md) doc for details on connecting a node with a manual installation of Netdata.

### 'nonrepresentable section on output' errors

Our current build process unfortunately has some issues when using certain configurations of the `clang` C compiler on Linux.

If the installation fails with errors like `/bin/ld: externaldeps/libwebsockets/libwebsockets.a(context.c.o): relocation R_X86_64_32 against '.rodata.str1.1' can not be used when making a PIE object; recompile with -fPIC`, and you are trying to build with `clang` on Linux, you will need to build Netdata using GCC to get a fully functional install. 

In most cases, you can do this by running `CC=gcc ./netdata-installer.sh`.


### Perform a cleanup in your netdata repo

The Netdata repo consist of the main git tree and it's submodules. Either working on a fork or on the main repo you need to make sure that there
are no "leftover" artifacts from previous builds and that your submodules are up to date to the **corresponding checkouts**.

> #### Important: Make sure that you have commited any work in progress, before you proceed the with the clean up instruction below


```sh
git clean -dfx && git submodule foreach 'git clean -dfx' && git submodule update --recursive --init
```


> Note: In previous builds, you may have created artifacts belonging to an another user (e.g root), so you may need to run
> each of the _git clean_ commands as sudoer.
