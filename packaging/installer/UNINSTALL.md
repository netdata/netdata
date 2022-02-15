<!--
title: "Uninstall Netdata"
description: "If you are no longer interested in using the Netdata Agent, use the self-contained uninstaller to remove all traces of binaries and configuration files."
custom_edit_url: https://github.com/netdata/netdata/edit/master/packaging/installer/UNINSTALL.md
-->

# Uninstall Netdata

> ⚠️ If you're having trouble updating Netdata, moving from one installation method to another, or generally having
> issues with your Netdata Agent installation, consider our [**reinstall Netdata**
> doc](/packaging/installer/REINSTALL.md) instead of removing the Netdata Agent entirely.

Our self-contained uninstaller is able to remove Netdata installations created with shell installer. It doesn't need any
other Netdata repository files to be run. All it needs is an `.environment` file, which is created during installation
(with shell installer) and put in `${NETDATA_USER_CONFIG_DIR}/.environment` (by default `/etc/netdata/.environment`).
That file contains some parameters which are passed to our installer and which are needed during uninstallation process.
Mainly two parameters are needed:

```sh
NETDATA_PREFIX
NETDATA_ADDED_TO_GROUPS
```

A workflow for uninstallation looks like this:

1.  Find your `.environment` file, which is usually `/etc/netdata/.environment` in a default installation.
2.  If you cannot find that file and would like to uninstall Netdata, then create a new file with the following content:

```sh
NETDATA_PREFIX="<installation prefix>"   # put what you used as a parameter to shell installed `--install` flag. Otherwise it should be empty
NETDATA_ADDED_TO_GROUPS="<additional groups>"  # Additional groups for a user running the Netdata process
```

3.  Run `netdata-uninstaller.sh` as follows

```sh
${NETDATA_PREFIX}/usr/libexec/netdata/netdata-uninstaller.sh --yes --env <environment_file>
```

Note: Existing installations may still need to download the file if it's not present. To execute uninstall in that case,
run the following commands:

```sh
wget https://raw.githubusercontent.com/netdata/netdata/master/packaging/installer/netdata-uninstaller.sh
chmod +x ./netdata-uninstaller.sh
./netdata-uninstaller.sh --yes --env <environment_file>
```

The default `environment_file` is `/etc/netdata/.environment`. 

> Note: This uninstallation method assumes previous installation with `netdata-installer.sh` or the kickstart script.
> Currently using it when Netdata was installed by a package manager can work or cause unexpected results.


