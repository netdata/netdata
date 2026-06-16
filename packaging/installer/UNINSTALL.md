# Uninstall Netdata

:::tip

**What You'll Learn**

How to uninstall Netdata from UNIX and Windows systems using automated scripts or manual methods based on your installation type.

:::

## Uninstall Methods by Platform

<details>
<summary><strong>UNIX</strong></summary><br/>

:::note

**Installation Method Note**

This method assumes you installed Netdata using the `kickstart.sh` or `netdata-installer.sh` script. If you used a different method, it might not work and could complicate the removal process.

:::

Similarly, with our documentation on updating Netdata, you need to [determine your installation type](/packaging/installer/UPDATE.md).

:::important

**Native Package Users**

If your installation type indicates a [native package](https://learn.netdata.cloud/docs/netdata-agent/installation/linux/native-linux-distribution-packages), then proceed to uninstall Netdata using your package manager.

:::

### Automated Uninstallation

The recommended way to uninstall Netdata is to use the same script you used for installation. Add the `--uninstall` flag:

```bash
wget -O /tmp/netdata-kickstart.sh https://get.netdata.cloud/kickstart.sh && sh /tmp/netdata-kickstart.sh --uninstall
```

<details>
<summary><strong>if you have curl but not wget</strong></summary><br/>

```sh
curl https://get.netdata.cloud/kickstart.sh > /tmp/netdata-kickstart.sh && sh /tmp/netdata-kickstart.sh --uninstall
```

<br/>
</details>

**What to Expect**:

In most cases, these commands will guide you through the uninstallation process and remove configuration and data files automatically.

**Non-Standard Installations**:

If you installed Netdata with a custom prefix (different directory location), you may need to specify the original prefix during uninstallation with the `--old-install-prefix` option.

### Uninstalling manually

Most official installations of Netdata include an uninstaller script that can be manually invoked instead of using the kickstart script (internally, the kickstart script also uses this uninstaller script, it just handles the process outlined below for you).

This uninstaller script is self-contained, other than requiring a `.environment` file that was generated during installation. In most cases, this will be found in `/etc/netdata/.environment`, though if you used a custom installation prefix, it will be located under that directory.

#### Manual Uninstallation Steps

1. **Find your `.environment` file**

2. **If you can't find that file and would like to uninstall Netdata, then create a new file with the following content:**

    ```sh
    NETDATA_PREFIX="<installation prefix>"   # put what you used as a parameter to shell installed `--install-prefix` flag. Otherwise it should be empty
    NETDATA_ADDED_TO_GROUPS="<additional groups>"  # Additional groups for a user running the Netdata process
    ```

3. **Run `netdata-uninstaller.sh` as follows**

    <details>
    <summary><strong>Interactive mode (Default)</strong></summary><br/>

   The default mode in the uninstaller script is **interactive**. This means that the script provides you the option to reply with "yes" (`y`/`Y`) or "no" (`n`/`N`) to control the removal of each Netdata asset in the filesystem.

    ```sh
    ${NETDATA_PREFIX}/usr/libexec/netdata/netdata-uninstaller.sh --yes --env <environment_file>
    ```

    <br/>
    </details>

    <details>
    <summary><strong>Non-interactive mode</strong></summary><br/>

   If you're sure, and you know what you're doing, you can speed up the removal of the Netdata assets from the filesystem without any questions by using the force option (`-f`/`--force`). This option will remove all the Netdata assets in a **non-interactive** mode.

    ```sh
    ${NETDATA_PREFIX}/usr/libexec/netdata/netdata-uninstaller.sh --yes --force --env <environment_file>
    ```

    <br/>
    </details>

:::note

**Missing Uninstaller File**

Existing installations may still need to download the file if it's not present. To execute the uninstaller in that case, run the following commands:

```sh
wget https://raw.githubusercontent.com/netdata/netdata/master/packaging/installer/netdata-uninstaller.sh
chmod +x ./netdata-uninstaller.sh
./netdata-uninstaller.sh --yes --env <environment_file>
```

:::

### Troubleshooting

#### dpkg errors during package purge

If you installed Netdata via native DEB packages (for example, `netdata`, `netdata-dashboard`, or `netdata-plugin-*`), the package's post-removal (postrm) script may fail during `purge` with output similar to the following:

```text
dpkg-statoverride: warning: no override present
dpkg: error processing package netdata-dashboard (--purge):
 installed netdata-dashboard package post-removal script subprocess returned error exit status 2
Errors were encountered while processing:
 netdata-dashboard
E: Sub-process /usr/bin/dpkg returned an error code (1)
```

The postrm script references `dpkg-statoverride` entries or file paths that no longer exist on the system, typically after a partial removal or manual cleanup. When this happens, the package is left in a half-configured state. The following steps resolve this, ordered from safest to most aggressive:

1. **Retry the purge** — the transient state may have resolved itself:

   ```bash
   sudo apt-get purge netdata-dashboard
   ```

   Replace `netdata-dashboard` with whichever Netdata package is failing.

2. **Remove stale dpkg-statoverride entries** — check for leftover overrides tied to Netdata:

   ```bash
   dpkg-statoverride --list | grep netdata
   ```

   For each path listed, remove the override:

   ```bash
   sudo dpkg-statoverride --remove <path>
   ```

   Then retry the purge:

   ```bash
   sudo apt-get purge netdata-dashboard
   ```

3. **Remove the broken postrm script** — if the postrm script itself is the problem:

   ```bash
   sudo rm /var/lib/dpkg/info/netdata-dashboard.postrm
   sudo dpkg --configure -a
   sudo apt-get purge netdata-dashboard
   ```

4. **Force removal** — last resort when nothing else works:

   ```bash
   sudo dpkg --remove --force-remove-reinstreq netdata-dashboard
   ```

:::note

These steps apply to any Netdata DEB package (`netdata`, `netdata-dashboard`, `netdata-plugin-*`) that uses `dpkg-statoverride` in its post-removal script. This scenario only affects native package installs — if you are unsure how Netdata was installed, see the **Native Package Users** note above and the [installation type guidance](/packaging/installer/UPDATE.md).

:::

<br/>
</details>

<details>
<summary><strong>Windows</strong></summary><br/>

To uninstall Netdata on Windows, use the standard application uninstaller in your **Settings** app or **Control Panel**.

You can also use PowerShell:

```powershell
msiexec /qn /x netdata-x64.msi
```

<br/>
</details>
