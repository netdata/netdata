<!--
title: "Install Netdata on Linux from a Git checkout"
description: "Use the Netdata Agent source code from GitHub, plus helper scripts to set up your system, to install Netdata without packages or binaries."
custom_edit_url: https://github.com/netdata/netdata/edit/master/packaging/installer/methods/manual.md
-->

# Install Netdata on Linux from a Git checkout

These instructions are for building netdata locally from a checkout of the git repository. They are primary of
interest to developers and contributors. Normal users are strongly encouraged to instead use [our official install
script](./kickstart.md).

## Check out the repository

To clone the repository, run:

```sh
git clone --recursive https://github.com/netdata/netdata
```

If your version of `git` does not support the `--recursive` option for the `clone` command, you can instead use:

```sh
git clone https://github.com/netdata/netdata
( cd netdata && git submodule update --init --recursive )
```

## Prepare your system

Netdata needs the following tools at build time:

-     autoconf
-     autoconf-archive
-     autogen
-     automake
-     CMake
-     GCC or a recent version of Clang
-     git
-     GNU make
-     G++ or a recent version of Clang++
-     gzip
-     libtool
-     pkg-config
-     tar
-     xz (Linux only)
-     cURL or wget

Headers and development files for the following libraries are also required at build time:

-     JSON-C
-     libatomic (Linux only)
-     libelf (Linux only)
-     libuuid
-     libuv 1.0 or newer
-     LZ4
-     OpenSSL 1.0.2 or newer or LibreSSL 3.0.0 or newer
-     zlib

We provide a script for handling required dependencies which is located at
`packaging/installer/install-required-packages.sh` in our Git repository. This script should work on most common
Linux distributons, as well as macOS and FreeBSD.  To use the automatic dependency handling script, you will need
GNU bash version 4.0 or newer.

To use this script from your checkout of the Netdata git repository, run the following from the root of the repository:

```sh
packaging/installer/install-required-packages.sh netdata
```

## Install Netdata

We provide a script that encapsulates the entire build and install process as a single command. You can run it
from the root of your checkout of the Netdata git repository like so:

```sh
./netdata-installer.sh
```

-   If you don't want to run it straight-away, add `--dont-start-it` option.
-   If you don't want to install it on the default directories, you can run the installer like this:
    `./netdata-installer.sh --install /opt`. This one will install Netdata in `/opt/netdata`.

## Optional parameters to alter your installation

`netdata-installer.sh` accepts a few parameters to customize your installation:

-   `--dont-wait`: Run without prompting for any user input.
-   `--dont-start-it`: Prevent the installer from starting Netdata automatically.
-   `--disable-telemetry`: Opt-out of [anonymous statistics](/docs/anonymous-statistics.md) we use to make
    Netdata better.

### Connect node to Netdata Cloud during installation

Unlike the [`kickstart.sh`](/packaging/installer/methods/kickstart.md), the `netdata-installer.sh` script does
not allow you to automatically [connect](/claim/README.md) your node to Netdata Cloud immediately after installation.

See the [connect to cloud](/claim/README.md) doc for details on connecting a node with a manual installation of Netdata.

### 'nonrepresentable section on output' errors

Our current build process unfortunately has some issues when using certain configurations of the `clang` C compiler on Linux.

If the installation fails with errors like `/bin/ld: externaldeps/libwebsockets/libwebsockets.a(context.c.o):
relocation R_X86_64_32 against '.rodata.str1.1' can not be used when making a PIE object; recompile with -fPIC`,
and you are trying to build with `clang` on Linux, you will need to build Netdata using GCC to get a fully
functional install.

In most cases, you can do this by running `CC=gcc ./netdata-installer.sh`.

## What's next?

When you're finished with installation, check out our [single-node](/docs/quickstart/single-node.md) or
[infrastructure](/docs/quickstart/infrastructure.md) monitoring quickstart guides based on your use case.

Or, skip straight to [configuring the Netdata Agent](/docs/configure/nodes.md).

Read through Netdata's [documentation](https://learn.netdata.cloud/docs), which is structured based on actions and
solutions, to enable features like health monitoring, alarm notifications, long-term metrics storage, exporting to
external databases, and more.
