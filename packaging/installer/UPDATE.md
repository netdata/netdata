# Update Netdata

By default, the Netdata Agent automatically updates with the latest nightly or stable version depending on which
you installed. If you opted out of automatic updates, you need to update your Netdata Agent to the latest nightly
or stable version. You can also [enable or disable automatic updates on an existing install](#control-automatic-updates).

> 💡 Looking to reinstall the Netdata Agent to enable a feature, update an Agent that cannot update automatically, or
> troubleshoot an error during the installation process? See our [reinstallation doc](https://github.com/netdata/netdata/blob/master/packaging/installer/REINSTALL.md)
> for reinstallation steps.

Before you update the Netdata Agent, check to see if your Netdata Agent is already up-to-date by clicking on the update
icon in the local Agent dashboard's top navigation. This modal informs you whether your Agent needs an update or not.

The exact update method to use depends on the install type:

-   Installs with an install type of 'custom' usually indicate installing a third-party package through the system
    package manager. To update these installs, you should update the package just like you would any other package
    on your system.
-   Installs with an install type starting with `binpkg` or ending with `build` or `static` can be updated using
    our [regular update method](#updates-for-most-systems).
-   Installs with an install type of 'oci' were created from our official Docker images, and should be updated
    using our [Docker](#docker) update procedure.
-   macOS users should check [our update instructions for macOS](#macos).
-   Manually built installs should check [our update instructions for manual builds](#manual-installation-from-git).

## Determine which installation method you used

Starting with netdata v1.33.0, you can use Netdata itself to determine the installation type by running:

```bash
netdata -W buildinfo | grep 'Install type:'
```

If you are using an older version of Netdata, or the above command produces no output, you can run our one-line
installation script in dry-run mode to attempt to determine what method to use to update by running the following
command:

```bash
wget -O /tmp/netdata-kickstart.sh https://my-netdata.io/kickstart.sh && sh /tmp/netdata-kickstart.sh --dry-run
```

Note that if you installed Netdata using an installation prefix, you will need to add an `--install-prefix` option
specifying that prefix to make sure it finds the existing install.

If you see a line starting with `--- Would attempt to update existing installation by running the updater script
located at:`, then our [regular update method](#updates-for-most-systems) will work for you.

Otherwise, it should either indicate that the installation type is not supported (which probably means you either
have a `custom` install or built Netdata manually) or indicate that it would create a new install (which means that
you either used a non-standard install path, or that you don’t actually have Netdata installed).

## Updates for most systems

In most cases, you can update netdata using our one-line installation script.  This script will automatically
run the update script that was installed as part of the initial install (even if you disabled automatic updates)
and preserve the existing install options you specified.

If you installed Netdata using an installation prefix, you will need to add an `--install-prefix` option specifying
that prefix to this command to make sure it finds Netdata.

```bash
wget -O /tmp/netdata-kickstart.sh https://my-netdata.io/kickstart.sh && sh /tmp/netdata-kickstart.sh
```

### Issues with older binpkg installs

The above command is known not to work with binpkg type installs for stable releases with a version number of
v1.33.1 or earlier, and nightly builds with a version number of v1.33.1-93 or earlier. If you have such a system,
the above command will report that it found an existing install, and then issue a warning about not being able to
find the updater script.

On such installs, you can update Netdata using your distribution package manager.

### If the kickstart script does not work

If the above command fails, you can [reinstall
Netdata](https://github.com/netdata/netdata/blob/master/packaging/installer/REINSTALL.md#one-line-installer-script-kickstartsh) to get the latest version. This
also preserves your [configuration](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md) in `netdata.conf` or other files just like updating
normally would, though you will need to specify any installation options you used originally again.

## Docker

Docker-based installations do not update automatically. To update an Netdata Agent running in a Docker container, you
must pull the [latest image from Docker Hub](https://hub.docker.com/r/netdata/netdata), stop and remove the container,
and re-create it using the latest image.

First, pull the latest version of the image.

```bash
docker pull netdata/netdata:latest
```

Next, to stop and remove any containers using the `netdata/netdata` image. Replace `netdata` if you changed it from the
default.

```bash
docker stop netdata
docker rm netdata
```

You can now re-create your Netdata container using the `docker` command or a `docker-compose.yml` file. See our [Docker
installation instructions](https://github.com/netdata/netdata/blob/master/packaging/docker/README.md#create-a-new-netdata-agent-container) for details.

## macOS

If you installed Netdata on your macOS system using Homebrew, you can explicitly request an update:

```bash
brew upgrade netdata
```

Homebrew downloads the latest Netdata via the
[formulae](https://github.com/Homebrew/homebrew-core/blob/master/Formula/netdata.rb), ensures all dependencies are met,
and updates Netdata via reinstallation.

If you instead installed Netdata using our one-line installation script, you can use our [regular update
instructions](#updates-for-most-systems) to update Netdata.

## Manual installation from Git

If you installed [Netdata manually from Git](https://github.com/netdata/netdata/blob/master/packaging/installer/methods/manual.md), you can run that installer again
to update your agent. First, run our automatic requirements installer, which works on many Linux distributions, to
ensure your system has the dependencies necessary for new features.

```bash
bash <(curl -sSL https://raw.githubusercontent.com/netdata/netdata/master/packaging/installer/install-required-packages.sh)
```

Navigate to the directory where you first cloned the Netdata repository, pull the latest source code, and run
`netdata-install.sh` again. This process compiles Netdata with the latest source code and updates it via reinstallation.

```bash
cd /path/to/netdata/git
git pull origin master
sudo ./netdata-installer.sh
```

> ⚠️ If you installed Netdata with any optional parameters, such as `--no-updates` to disable automatic updates, and
> want to retain those settings, you need to set them again during this process.

## Control automatic updates

Starting with Netdata v1.34.0, you can easily enable or disable automatic updates on an existing installation
using the updater script.

For most installs on Linux, you can enable auto-updates with:

```bash
/usr/libexec/netdata/netdata-updater.sh --enable-auto-updates
```

and disable them with:

```bash
/usr/libexec/netdata/netdata-updater.sh --disable-auto-updates
```

For static installs, instead use:

```bash
/opt/netdata/usr/libexec/netdata/netdata-updater.sh --enable-auto-updates
```

and:

```bash
/opt/netdata/usr/libexec/netdata/netdata-updater.sh --disable-auto-updates
```
