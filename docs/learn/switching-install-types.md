# Switching Netdata Install Types and Release Channels

You can switch between different Netdata installation types and release channels based on your needs. This guide covers both scenarios with step-by-step instructions.

## Switching Install Types

### Data Backups When Switching Install Types

Before switching install types, you need to back up your configuration and data to preserve your settings.

<details>
<summary><strong>For Native Packages and Local Builds</strong></summary><br/>

When backing up configuration and data for install types other than static builds or Docker containers, you should back up these directories:

- `/etc/netdata` (excluding `/etc/netdata/.environment` and `/etc/netdata/.install-type`)
- `/var/cache/netdata`
- `/var/lib/netdata`

:::warning

You must exclude the `.environment` and `.install-type` files from your backup. Copying these files from one install type to another will break updates.

:::

</details>

<br/>

<details>
<summary><strong>For Docker Containers</strong></summary><br/>

When backing up configuration and data for Docker containers, you should back up these paths from inside the container:

- `/etc/netdata` (excluding `/etc/netdata/.environment` and `/etc/netdata/.install-type`)
- `/var/cache/netdata`
- `/var/lib/netdata`

:::warning

You must exclude the `.environment` and `.install-type` files from your backup. Copying these files from one install type to another will break updates.

:::

</details>

<br/>

<details>
<summary><strong>For Static Builds</strong></summary><br/>

When backing up configuration and data for static builds, you should back up these directories:

- `/opt/netdata/etc/netdata` (excluding `/opt/netdata/etc/netdata/.environment` and `/opt/netdata/etc/netdata/.install-type`)
- `/opt/netdata/var/cache/netdata`
- `/opt/netdata/var/lib/netdata`

:::warning

You must exclude the `.environment` and `.install-type` files from your backup. Copying these files from one install type to another will break updates.

:::

</details>

<br/>

<details>
<summary><strong>Switching Between Install Types (Non-Docker)</strong></summary><br/>

For all install types other than Docker images, you can use this officially supported method:

1. **Back up your configuration and data** that you want to preserve using the appropriate method above.

2. **Run the kickstart script with clean reinstall:**

   ```bash
   wget -O /tmp/netdata-kickstart.sh https://get.netdata.cloud/kickstart.sh
   sh /tmp/netdata-kickstart.sh --reinstall-clean [OPTIONS_FOR_DESIRED_INSTALL_TYPE]
   ```

3. **Restore your backups** to the appropriate paths in the new installation.

</details>

<br/>

<details>
<summary><strong>Switching From Native/Static/Local to Docker</strong></summary><br/>

To switch from a native, static, or local install to a Docker image:

1. **Back up your configuration and data** that you want to preserve.

2. **Uninstall the existing installation:**

   ```bash
   wget -O /tmp/netdata-kickstart.sh https://get.netdata.cloud/kickstart.sh
   sh /tmp/netdata-kickstart.sh --uninstall
   ```

3. **Start the Docker container** for the Netdata agent.

4. **Restore your backup contents** to the respective paths inside the Docker container (the paths within the container are the same as those used for a native package install on the host).

5. **Restart the Docker container** to apply the restored configuration.

</details>

<br/>

<details>
<summary><strong>Switching From Docker to Native/Static/Local</strong></summary><br/>

To switch from a Docker container to a different install type:

1. **Back up your configuration and data** that you want to preserve from inside the container.

2. **Remove the container.**

3. **Install Netdata using the kickstart script:**

   ```bash
   wget -O /tmp/netdata-kickstart.sh https://get.netdata.cloud/kickstart.sh
   sh /tmp/netdata-kickstart.sh [OPTIONS_FOR_DESIRED_INSTALL_TYPE]
   ```

4. **Restore your backups** to the appropriate paths in the new installation.

</details>

## Switching Between Release Channels

### Static Builds and Local Builds

For static builds and local builds, you can simply run the kickstart script with the reinstall option:

<details>
<summary><strong>Switch to Stable Channel</strong></summary><br/>

```bash
wget -O /tmp/netdata-kickstart.sh https://get.netdata.cloud/kickstart.sh
sh /tmp/netdata-kickstart.sh --reinstall --stable-channel
```

</details>

<br/>

<details>
<summary><strong>Switch to Nightly Channel</strong></summary><br/>

```bash
wget -O /tmp/netdata-kickstart.sh https://get.netdata.cloud/kickstart.sh
sh /tmp/netdata-kickstart.sh --reinstall --nightly-channel
```

</details>

### Docker Containers

Follow the standard container recreation process used for updates, but replace the image tag with the one corresponding to your target release channel:

- **Stable**: Use the `stable` tag
- **Nightly**: Use the `latest` tag

### Native Packages

#### Debian/Ubuntu

<details>
<summary><strong>Switch from Nightly to Stable</strong></summary><br/>

```bash
# Install stable repository (automatically removes nightly repo)
sudo apt install netdata-repo
sudo apt update
sudo apt remove netdata
sudo apt install netdata
```

</details>

<br/>

<details>
<summary><strong>Switch from Stable to Nightly</strong></summary><br/>

```bash
# Install nightly repository (automatically removes stable repo)
sudo apt install netdata-repo-edge
sudo apt update
sudo apt remove netdata
sudo apt install netdata
```

</details>

<br/>

#### Fedora/RHEL

<details>
<summary><strong>Switch from Nightly to Stable</strong></summary><br/>

```bash
# Install stable repository (automatically removes nightly repo)
sudo dnf install --allowerasing netdata-repo
sudo dnf remove netdata
sudo dnf install --refresh netdata
```

</details>

<br/>

<details>
<summary><strong>Switch from Stable to Nightly</strong></summary><br/>

```bash
# Install nightly repository (automatically removes stable repo)
sudo dnf install --allowerasing netdata-repo-edge
sudo dnf remove netdata
sudo dnf install --refresh netdata
```

</details>

<br/>

#### openSUSE

<details>
<summary><strong>Switch from Nightly to Stable</strong></summary><br/>

```bash
# Install stable repository (automatically removes nightly repo)
sudo zypper install --allowerasing netdata-repo
sudo zypper refresh
sudo zypper remove netdata
sudo zypper install netdata
```

</details>

<br/>

<details>
<summary><strong>Switch from Stable to Nightly</strong></summary><br/>

```bash
# Install nightly repository (automatically removes stable repo)
sudo zypper install --allowerasing netdata-repo-edge
sudo zypper refresh
sudo zypper remove netdata
sudo zypper install netdata
```

</details>

<br/>

:::tip

When switching release channels with native packages, the repository configuration packages automatically handle the removal of the previous channel's repository to prevent conflicts.

:::
