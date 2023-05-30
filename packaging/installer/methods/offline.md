<!--
title: "Install Netdata on offline systems"
description: "Install the Netdata Agent on offline/air gapped systems to benefit from real-time, per-second monitoring without connecting to the internet."
custom_edit_url: "https://github.com/netdata/netdata/edit/master/packaging/installer/methods/offline.md"
sidebar_label: "Offline systems"
learn_status: "Published"
learn_rel_path: "Installation/Installation methods"
sidebar_position: 50
-->

# Install Netdata on offline systems

Our kickstart install script provides support for installing the Netdata Agent on air-gapped systems which do not have a
usable internet connection by prefetching all of the required files so that they can be copied to the target system.
Currently, we only support using static installs with this method. There are tentative plans to support building
locally on offline systems as well, but there is currently no estimate of when this functionality may be implemented.

Users who wish to use native packages on offline systems may be able to do so using whatever tooling their
distribution already provides for offline package management (such as `apt-offline` on Debian or Ubuntu systems),
but this is not officially supported.

## Preparing the offline installation source

The first step to installing Netdata on an offline system is to prepare the offline installation source. This can
be as a regular user from any internet connected system that has the following tools available:

- cURL or wget
- sha256sum or shasum
- A standard POSIX compliant shell

To prepare the offline installation source, simply run:

```bash
wget -O /tmp/netdata-kickstart.sh https://my-netdata.io/kickstart.sh && sh /tmp/netdata-kickstart.sh --prepare-offline-install-source ./netdata-offline
```

or

```bash
curl https://my-netdata.io/kickstart.sh > /tmp/netdata-kickstart.sh && sh /tmp/netdata-kickstart.sh --prepare-offline-install-source ./netdata-offline
```

> The exact name used for the directory does not matter, you can specify any other name you want in place of `./netdata-offline`.

This will create a directory called `netdata-offline` in the current directory and place all the files required for an offline install in it.

If you want to use a specific release channel (nightly or stable), it _must_ be specified on this step using the
appropriate option for the kickstart script.

## Installing on the target system

Once you have prepared the offline install source, you need to copy the offline install source directory to the
target system. This can be done in any manner you like, as long as filenames are not changed.

After copying the files, simply run the `install.sh` script located in the
offline install source directory. It accepts all the [same options as the kickstart
script](https://github.com/netdata/netdata/blob/master/packaging/installer/methods/kickstart.md#optional-parameters-to-alter-your-installation) for further
customization of the installation, though it will default to not enabling automatic updates (as they are not
supported on offline installs).
