# Install Netdata on offline systems


Our kickstart script enables offline Netdata Agent installation by prefetching required files (static installs only for now). Future support for local builds is possible, but no ETA. For offline systems, users can try their distro’s package tools (e.g., `apt-offline`), though it's not officially supported.

## Preparing the offline installation source

The first step in installing Netdata on an offline system is preparing the offline installation source. This can be done as a regular user on any internet-connected system with the following tools available:

- cURL or wget
- sha256sum or shasum
- A standard POSIX compliant shell

To prepare the offline installation source, simply run:

```bash
wget -O /tmp/netdata-kickstart.sh https://get.netdata.cloud/kickstart.sh && sh /tmp/netdata-kickstart.sh --prepare-offline-install-source ./netdata-offline
```

or

```bash
curl https://get.netdata.cloud/kickstart.sh > /tmp/netdata-kickstart.sh && sh /tmp/netdata-kickstart.sh --prepare-offline-install-source ./netdata-offline
```

> The exact name used for the directory does not matter, you can specify any other name you want in place of `./netdata-offline`.

This will create a directory called `netdata-offline` in the current directory and place all the files required for an offline install in it.

If you want to use a specific release channel (nightly or stable), it _must_ be specified on this step using the
appropriate option for the kickstart script.

## Installing on the target system

Once you have prepared the offline install source, you need to copy the offline install source directory to the target system. This can be done in any manner you like, as long as filenames are not changed.

After copying the files, simply run the `install.sh` script located in the
offline install source directory. It accepts all the [same options as the kickstart script](/packaging/installer/methods/kickstart.md#optional-parameters-to-alter-your-installation) for further customization of the installation, though it will default to not enabling automatic updates (as they are not supported on offline installs).