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
