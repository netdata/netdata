# Uninstall Netdata

> ### Note 
> 
> If you're having trouble updating Netdata, moving from one installation method to another, or generally having
> issues with your Netdata Agent installation, consider our [reinstalling Netdata](https://github.com/netdata/netdata/blob/master/packaging/installer/REINSTALL.md) instead of removing the Netdata Agent entirely.

The recommended method to uninstall Netdata on a system is to use our kickstart installer script with the `--uninstall` option like so:

```sh
wget -O /tmp/netdata-kickstart.sh https://my-netdata.io/kickstart.sh && sh /tmp/netdata-kickstart.sh --uninstall
```

Or (if you have curl but not wget):

```sh
curl https://my-netdata.io/kickstart.sh > /tmp/netdata-kickstart.sh && sh /tmp/netdata-kickstart.sh --uninstall
```

This will work in most cases without you needing to do anything more other than accepting removal of configuration
and data files.

If you used a non-standard installation prefix, you may need to specify that prefix using the `--old-install-prefix`
option when uninstalling this way.

## Unofficial installs

If you used a third-party package to install Netdata, then the above method will usually not work, and you will
need to use whatever mechanism you used to originally install Netdata to uninstall it.

## Uninstalling manually

Most official installs of Netdata include an uninstaller script that can be manually invoked instead of using the
kickstart script (internally, the kickstart script also uses this uninstaller script, it just handles the process
outlined below for you).

This uninstaller script is self-contained other than requiring a `.environment` file that was generated during
installation. In most cases, this will be found in `/etc/netdata/.environment`, though if you used a non-standard
installation prefix it will usually be located in a similar place under that prefix.

A workflow for uninstallation looks like this:

1.  Find your `.environment` file, which is usually `/etc/netdata/.environment` in a default installation.
2.  If you cannot find that file and would like to uninstall Netdata, then create a new file with the following content:

```sh
NETDATA_PREFIX="<installation prefix>"   # put what you used as a parameter to shell installed `--install-prefix` flag. Otherwise it should be empty
NETDATA_ADDED_TO_GROUPS="<additional groups>"  # Additional groups for a user running the Netdata process
```

3.  Run `netdata-uninstaller.sh` as follows

    3.1 **Interactive mode (Default)**

    The default mode in the uninstaller script is **interactive**. This means that the script provides you
    the option to reply with "yes" (`y`/`Y`) or "no" (`n`/`N`) to control the removal of each Netdata asset in
    the filesystem.

    ```sh
    ${NETDATA_PREFIX}/usr/libexec/netdata/netdata-uninstaller.sh --yes --env <environment_file>
    ```

    3.2 **Non-interactive mode**

    If you are sure and you know what you are doing, you can speed up the removal of the Netdata assets from the
    filesystem without any questions by using the force option (`-f`/`--force`). This option will remove all the
    Netdata assets in a **non-interactive** mode.

    ```sh
    ${NETDATA_PREFIX}/usr/libexec/netdata/netdata-uninstaller.sh --yes --force --env <environment_file>
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
> Using it when Netdata was installed in some other way will usually not work correctly, and may make it harder to uninstall Netdata.
