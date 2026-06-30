# Update Netdata

:::tip

**What You'll Learn**

How to update Netdata based on your installation method, determine your installation type, and configure the updater behavior.

:::

The update process can differ based on the installation type:

- Install types starting with `binpkg` or ending with `build` or `static` can be updated using our [kickstart script update method](#update-methods-by-platform).
- Installs with an installation type of `custom` usually indicate installing a third-party package through the system package manager. To update these installations, you should update the package just like you would any other package on your system.
- macOS users should check out [our update instructions for macOS](#update-methods-by-platform).
- Manually built installs should check out [our update instructions for manual builds](#update-methods-by-platform).

## Determine which installation method you used

:::important

**First Step**

Before updating, you need to identify your installation type to choose the correct update method.

:::

You can run the following to determine your installation type:

```bash
netdata -W buildinfo | grep -E 'Installation Type|Install type:'
```

:::tip

**If the above command doesn't work**

If you're using an older Netdata version or the above command doesn't output anything, try our one-line installation script in dry-run mode. Run the following command to determine the appropriate update method:

```bash
wget -O /tmp/netdata-kickstart.sh https://get.netdata.cloud/kickstart.sh && sh /tmp/netdata-kickstart.sh --dry-run
```

:::note

**Installation Prefix**

If you installed Netdata using an installation prefix, you will need to add an `--install-prefix` option specifying that prefix to make sure it finds the existing installation.

If you see a line starting with `--- Would attempt to update existing installation by running the updater script located at:`, then our [kickstart script update method](#update-methods-by-platform) will work for you.

Otherwise, it should either indicate that the installation type is not supported (which probably means you either have a `custom` install or built Netdata manually) or indicate that it would create a new install (which means that you either used a non-standard installation path, or that you don't have Netdata installed).

:::

The following table contains all possible installation types:

| Installation-type  | Description                                                                                                                                                 |
|--------------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------|
| binpkg-rpm         | RPM-based native packages shipped from Netdata's repos.                                                                                                     |
| binpkg-deb         | DEB-based native packages shipped from Netdata's repos.                                                                                                     |
| kickstart-build    | Build from source with the kickstart script's `--build-only` option.                                                                                        |
| kickstart-static   | Installed the static builds, shipped from netdata via the kickstart script's (option: `--static-only`).                                                     |
| manual-static-ARCH | Manually installed static Agent binaries by downloading archives from GitHub and installing them manually. Offline installations are part of this category. |
| legacy-build       | Used for pre-existing kickstart.sh or netdata-installer.sh installations. This exist because we cannot determine how the install originally happened.       |
| legacy-static      | Same as legacy-build, but for static installs.                                                                                                              |
| oci                | Installed using official Docker images from Netdata, though not necessarily running on Docker                                                               |
| custom             | Anything not covered by the other identifiers, including manual builds, manually running netdata-installer.sh, and third-party packages (community).        |
| Unknown            | Same as custom.                                                                                                                                             |

## Update Methods by Platform

<details>
<summary><strong>UNIX</strong></summary><br/>

In most cases, you can update Netdata using our one-line kickstart script. This script will automatically run the update script installed as part of the initial install and preserve the existing installation options you specified.

If you installed Netdata using an installation prefix, you will need to add an `--install-prefix` option specifying that prefix to this command to make sure it finds Netdata.

```bash
wget -O /tmp/netdata-kickstart.sh https://get.netdata.cloud/kickstart.sh && sh /tmp/netdata-kickstart.sh
```

<br/>
</details>

<details>
<summary><strong>Windows</strong></summary><br/>

To update Netdata, [download](/packaging/windows/WINDOWS_INSTALLER.md#download-the-windows-installer-msi) the latest installer and reinstall the Agent.

For automatic updates, see our [Windows automatic updates guide](https://learn.netdata.cloud/docs/netdata-agent/installation/windows#automatic-updates).

<br/>
</details>

<details>
<summary><strong>macOS</strong></summary><br/>

If you installed Netdata on your macOS system using Homebrew, you can explicitly request an update:

```bash
brew upgrade netdata
```

Homebrew downloads the latest Netdata via the [formula](https://github.com/Homebrew/homebrew-core/blob/master/Formula/n/netdata.rb), ensures all dependencies are met, and updates Netdata via reinstallation.

<br/>
</details>

<details>
<summary><strong>Manual installation from Git</strong></summary><br/>

If you installed [Netdata manually from Git](/packaging/installer/methods/manual.md) run our automatic requirements installer, which works on many Linux distributions, to ensure your system has the dependencies necessary for new features.

```bash
bash <(curl -sSL https://raw.githubusercontent.com/netdata/netdata/master/packaging/installer/install-required-packages.sh)
```

Navigate to the directory where you first cloned the Netdata repository, pull the latest source code, and run `netdata-install.sh` again. This process compiles Netdata with the latest source code and updates it via reinstallation.

```bash
cd /path/to/netdata/git
git pull origin master
sudo ./netdata-installer.sh
```

:::note

**Optional Parameters**

If you installed Netdata with any optional parameters, such as `--install-prefix` to install under a specific directory, you need to set them again during this process.

:::

<br/>
</details>

<details>
<summary><strong>Docker (OCI)</strong></summary><br/>

Docker-based installations are updated by pulling a new image and recreating the container. Persistent volumes preserve your configuration and metrics data across updates.

For detailed instructions, see [Update your Netdata Docker container](/packaging/docker/README.md#update-your-netdata-docker-container).

<br/>
</details>

## Additional Configuration

### Control runtime behavior of the updater script

Starting with v1.40.0, the `netdata-updater.sh` script supports a config file called `netdata-updater.conf`, located in the same directory as the main `netdata.conf` file. This file uses POSIX shell script syntax to define variables that are used by the updater.

This configuration file can be edited using our [`edit-config` script](/docs/netdata-agent/configuration/README.md).

**Available Configuration Options:**

| Option                          | Description                                                                                                                                                                                                                                                                                      | Default         |
|---------------------------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|-----------------|
| `NETDATA_UPDATER_JITTER`        | Sets an upper limit in seconds on the random delay in the updater script when running as a scheduled task. This random delay helps avoid issues resulting from too many nodes trying to reconnect to the Cloud at the same time. Most users shouldn't ever need to change this.                  | 3600 (one hour) |
| `NETDATA_ACCEPT_MAJOR_VERSIONS` | A space-separated list of major versions the updater installs automatically without prompting. An empty value uses the updater's built-in default (currently `1 2`). To stay on a specific major version, set this to that version number (for example, `1`). Applies to all install types managed by the auto-updater, including native packages. | Empty (script defaults to `1 2`) |
| `NETDATA_NO_SYSTEMD_JOURNAL`    | If set to a value other than 0, skip attempting to install the `netdata-plugin-systemd-journal` package on supported systems on update. The updater will install this optional package by default on supported systems if this option is not set. It only affects systems using native packages. | 0               |

## Managing Automatic Updates

Netdata enables daily auto-updates by default when installed using the kickstart script (unless you pass `--no-updates` during installation). The schedule runs once per day. The installer auto-detects the scheduling method, which may be a cron entry under `/etc/cron.daily` or `/etc/periodic/daily`, the `netdata-updater.timer` systemd unit (`OnCalendar=daily`), or a crontab under `/etc/cron.d`.

### Disable auto-updates

<details>
<summary><strong>At install time (kickstart)</strong></summary><br/>

Pass `--no-updates` to the kickstart script to skip setting up auto-updates entirely:

```bash
wget -O /tmp/netdata-kickstart.sh https://get.netdata.cloud/kickstart.sh && sh /tmp/netdata-kickstart.sh --no-updates
```

To explicitly control the scheduling method, use `--auto-update-type` with one of `systemd`, `interval`, or `crontab`:

```bash
wget -O /tmp/netdata-kickstart.sh https://get.netdata.cloud/kickstart.sh && sh /tmp/netdata-kickstart.sh --auto-update-type systemd
```

<br/>
</details>

The simplest method on an existing installation is the updater script's built-in command, which removes every supported scheduling method (systemd timer, `/etc/cron.daily`, `/etc/periodic/daily`, and `/etc/cron.d`) in one step:

```bash
sudo /usr/libexec/netdata/netdata-updater.sh --disable-auto-updates
```

This takes effect immediately — the systemd timer is stopped and any cron entries are removed, so no further automatic updates will run.

If you prefer to disable the scheduler manually:

- **systemd:** `sudo systemctl disable --now netdata-updater.timer` (stops and disables the timer unit).
- **non-systemd (cron):** remove the entry your installer created. Remove whichever exists:
  ```bash
  sudo rm -f /etc/cron.daily/netdata-updater /etc/cron.daily/netdata-updater.sh
  sudo rm -f /etc/periodic/daily/netdata-updater /etc/periodic/daily/netdata-updater.sh
  sudo rm -f /etc/cron.d/netdata-updater /etc/cron.d/netdata-updater-daily
  ```
  Only one of these directories will contain a file; removing a non-existent path is harmless.

:::note

The path `/usr/libexec/netdata/netdata-updater.sh` is for a standard install with the default prefix. If you installed with `--install-prefix`, the script lives under `<your-prefix>/usr/libexec/netdata/netdata-updater.sh`.

:::

:::note

**Native package installations**

`--disable-auto-updates` stops the Netdata-managed update scheduler for all install types, including native packages. However, system-level auto-upgrade tools (such as `unattended-upgrades` on Debian/Ubuntu or `dnf-automatic` on RHEL-based systems) can still update Netdata independently of the Netdata scheduler. To prevent that, also pin the package using your system's package manager:

```bash
# DEB-based systems
sudo apt-mark hold netdata

# RPM-based systems (requires dnf-plugins-core)
sudo dnf versionlock add netdata
```

:::

### Check auto-update status

```bash
sudo /usr/libexec/netdata/netdata-updater.sh --auto-update-status
```

This reports which scheduling methods are detected as enabled or disabled on your system.

### Re-enable auto-updates

```bash
sudo /usr/libexec/netdata/netdata-updater.sh --enable-auto-updates
```

The script auto-detects the appropriate scheduler for your system. To explicitly set the method, pass it as an argument:

```bash
sudo /usr/libexec/netdata/netdata-updater.sh --enable-auto-updates systemd
```

Valid methods are `systemd`, `interval`, and `crontab`.

:::warning

`netdata-updater.conf` controls **how** the updater runs, not **whether** it runs — see the [configuration options](#control-runtime-behavior-of-the-updater-script) above. It does not contain an option to disable the auto-update schedule, and setting variables in it will not stop auto-updates. To turn auto-updates off, use the disable command above — not the config file.

:::


## Quick Reference

### Update Commands by Installation Type

| Installation Type          | Update Method          | Command                                                                                                    |
|----------------------------|------------------------|------------------------------------------------------------------------------------------------------------|
| **binpkg-rpm/deb**         | Kickstart script       | `wget -O /tmp/netdata-kickstart.sh https://get.netdata.cloud/kickstart.sh && sh /tmp/netdata-kickstart.sh` |
| **kickstart-build/static** | Kickstart script       | `wget -O /tmp/netdata-kickstart.sh https://get.netdata.cloud/kickstart.sh && sh /tmp/netdata-kickstart.sh` |
| **legacy-build/static**    | Kickstart script       | `wget -O /tmp/netdata-kickstart.sh https://get.netdata.cloud/kickstart.sh && sh /tmp/netdata-kickstart.sh` |
| **manual-static-ARCH**     | Kickstart script       | `wget -O /tmp/netdata-kickstart.sh https://get.netdata.cloud/kickstart.sh && sh /tmp/netdata-kickstart.sh` |
| **custom**                 | System package manager | Use your system's package manager                                                                          |
| **macOS (Homebrew)**       | Homebrew               | `brew upgrade netdata`                                                                                     |
| **Manual Git**             | Git + installer        | See [manual installation steps](#update-methods-by-platform)                                               |
| **Docker (OCI)**           | Image pull + recreate  | See [Docker update instructions](/packaging/docker/README.md#update-your-netdata-docker-container)         |
| **Windows**                | MSI installer          | Download and run latest installer                                                                          |
