# Other installation methods

The Netdata team works hard to make Netdata installable on as many systems as possible. This page contains instructions for some less common operating systems or those with different installation requirements, in addition to certain machines that require unique configurations.

If you're installing Netdata on a Linux system, we recommend you try our [one-line automatic installation](README.md#one-line-installation) or [binary releases](README.md#binary-packages) first. If those don't work, you can try the [pre-built static binary](#pre-built-static-binary-for-linux-64-bit) or the [manual installation](MANUAL-INSTALLATION.md).

macOS and FreeBSD users can find instructions for their operating systems below.

- [Pre-built static binary for Linux 64-bit](#pre-built-static-binary-for-linux-64-bit)
- [macOS](#macos)
- [FreeBSD](#freebsd)
- [pfSense](#pfsense)
- [FreeNAS](#freenas)
- [Alpine 3.x](#alpine-3-x)
- [Synology](#synology)


## Pre-built static binary for Linux 64-bit
![](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.requests_per_url&options=unaligned&dimensions=kickstart64&group=sum&after=-3600&label=last+hour&units=installations&value_color=orange&precision=0) ![](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.requests_per_url&options=unaligned&dimensions=kickstart64&group=sum&after=-86400&label=today&units=installations&precision=0)

You can install a pre-compiled static binary of Netdata on any Intel/AMD 64-bit Linux system—even those that don't have a package manager, like CoreOS, CirrOS, busybox systems, among others. You can also use these packages on systems with broken or unsupported package managers.

To install Netdata from a binary package on any Linux distro and any kernel version on **Intel/AMD 64bit** systems, and get **automatic, nightly** updates, run the following:

```bash
$ bash <(curl -Ss https://my-netdata.io/kickstart-static64.sh)
```

!!! note "Usage notes"
    Do not use `sudo` for this installer—it will escalate privileges itself if needed.

    To learn more about the pros and cons of using *nightly* vs. *stable* releases, see our [notice about the two options](README.md#nightly-vs-stable-releases).

    If your system does not have `bash` installed, open the `More information and advanced uses of the kickstart-static64.sh script` dropdown for instructions to run the installer without `bash`.

    This script installs Netdata at `/opt/netdata`.

<details markdown="1"><summary>More information and advanced uses of the kickstart-static64.sh script</summary>

**What `kickstart-static64.sh` does:**

The `kickstart-static64.sh` script:

- Detects the Linux distro and installs the required system packages for building Netdata after asking for confirmation
- Downloads the latest Netdata source tree to `/usr/src/netdata.git`
- Installs Netdata at `/opt/netdata` by running `./netdata-installer.sh` from the source tree
- Installs `netdata-updater.sh` to `cron.daily`, so your Netdata installation will be updated daily
- Outputs details about whether the installation succeeded or failed.

**Available options:**

You can customize your Netdata installation by passing options from `kickstart-static64.sh` to `netdata-installer.sh`. With these options you can change the installation directory, enable/disable automatic updates, choose between the nightly (default) or stable channel, enable/disable plugins, and much more. For a full list of options, see the [`netdata-installer.sh` script](https://github.com/netdata/netdata/netdata-installer.sh#L149-L177).

Here are a few popular options:

- `--stable-channel`: Automatically update only on the release of new major versions.
- `--no-updates`: Prevent automatic updates of any kind.
- `--dont-wait`: Enable automated installs by not prompting for permission to install any required packages.
- `--dont-start-it`: Prevent the installer from starting Netdata automatically.

Here's an example of how to pass a few options through `kickstart-static64.sh`:

```bash
bash <(curl -Ss https://my-netdata.io/kickstart-static64.sh) --dont-wait --dont-start-it --stable-channel
```

**Verify the script's integrity:**

Verify the integrity of the script with this:

```bash
[ "8779d8717ccaa8dac18d599502eef591" = "$(curl -Ss https://my-netdata.io/kickstart-static64.sh | md5sum | cut -d ' ' -f 1)" ] && echo "OK, VALID" || echo "FAILED, INVALID"
```

This command will output `OK, VALID` to confirm that the script is intact and has not been tampered with.

**If your shell fails to handle the `kickstart-static64.sh` script:**

If the one-line installation script fails—for example, if you do not have `bash` installed—you can use the following commands to download the scripts and run them manually:

```bash
# download the script with curl
curl https://my-netdata.io/kickstart-static64.sh >/tmp/kickstart-static64.sh

# or, download the script with wget
wget -O /tmp/kickstart-static64.sh https://my-netdata.io/kickstart-static64.sh

# run the downloaded script (any sh is fine, no need for bash)
sh /tmp/kickstart-static64.sh
```

The installation script will let you know if it was successful or not.

You can also download staic binary files from our [stable releases page](https://github.com/netdata/netdata/releases) or grab the [nightly .run binary](https://storage.googleapis.com/netdata-nightlies/netdata-latest.gz.run). These `.run` files are self-extracting shell scripts build with [makeself](https://github.com/megastep/makeself).

With either of these methods, your system does not need `bash` installed. You can use either of these methods to update your Netdata installation as well.

</details>

Once you have installed Netdata, see our [getting started guide](../../docs/GettingStarted.md).


## macOS

Netdata on macOS still has limited charts, but external plugins do work.

You can install Netdata with [Homebrew](https://brew.sh/) or from source. In both cases, you need to have Homebrew installed. They offer a one-line installation script:

```bash
/usr/bin/ruby -e "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/master/install)"
```

Now that you have Homebrew installed, you can choose between continuing to use Homebrew to install Netdata, or install from source.

### Install Netdata via Homebrew

Homebrew makes installing Netdata quite easy. The entire installation package is wrapped up in a single command:

```bash
brew install netdata
```

You're done! For more information on how to use and configure Netdata, see our [getting started guide](../../docs/GettingStarted.md).


### Install Netdata from source

To install Netdata from source, begin by installing Xcode command line tools.

```bash
xcode-select --install
```

Click `Install` in the software update popup window. Once the update is completed, run the following commands to install prerequisities and then Netdata itself:

```sh
# Install required packages
brew install ossp-uuid autoconf automake pkg-config

# Hownload Netdata
git clone https://github.com/netdata/netdata.git --depth=100

# Install Netdata in /usr/local/netdata
cd netdata
sudo ./netdata-installer.sh --install /usr/local
```

The installer will also install a startup plist to start Netdata when your macOS system boots.

The installation from source should now be finished successfully. For more information on how to use and configure Netdata, see our [getting started guide](../../docs/GettingStarted.md).


## FreeBSD

You can install Netdata from the ports or packages collections. This is how to install the latest Netdata version from source on FreeBSD:

```sh
# Install required packages
pkg install bash e2fsprogs-libuuid git curl autoconf automake pkgconf pidof

# Download Netdata
git clone https://github.com/netdata/netdata.git --depth=100

# Install Netdata in /opt/netdata
cd netdata
./netdata-installer.sh --install /opt
```


## pfSense

To install Netdata on [pfSense](https://www.pfsense.org/), run the following commands within a shell or under the Diagnostics/Command Prompt within the pfSense web interface.

Change platform (i386/amd64, etc) and FreeBSD versions (10/11, etc) according to your environment and change Netdata version (1.15.0 in the example below) according to latest version present within the FreeSBD repository.

The first three packages are downloaded from the pfSense repository for maintaining compatibility with pfSense, but Netdata is downloaded from the FreeBSD repository.

```
pkg install pkgconf
pkg install bash
pkg install e2fsprogs-libuuid
pkg add http://pkg.freebsd.org/FreeBSD:11:amd64/latest/All/python36-3.6.9.txz
pkg add http://pkg.freebsd.org/FreeBSD:11:amd64/latest/All/netdata-1.15.0.txz
```

To start Netdata manually, run `service netdata onestart`.

To start Netdata automatically at each boot, add `service netdata onestart` as a Shellcmd within the pfSense web interface (under `Services/Shellcmd`, which you need to install beforehand under `System/Package Manager/Available Packages`).

The `Shellcmd Type` field should be set to `shellcmd`:

![](https://i.imgur.com/wcKiPe1.png)

More information can be found in the pfSense documentation on [installing FreeBSD packages](https://doc.pfsense.org/index.php/Installing_FreeBSD_Packages).

If you experience an issue with `/usr/bin/install` being absent from your system on pfSense 2.3 or earlier, update pfSense or use the workaround from [the pfSense issues](https://redmine.pfsense.org/issues/6643)  

!!! note
    The Netdata configuration files will be located under `/usr/local/etc/netdata`.


## FreeNAS

On FreeNAS-Corral-RELEASE (>=10.0.3), Netdata is pre-installed.

To use Netdata, you need to enable and start the service from the FreeNAS **[CLI](https://github.com/freenas/cli)**.

To enable the Netdata service:

```
service netdata config set enable=true
```

To start the netdata service:

```
service netdata start
```


## Alpine 3.x

Execute these commands to install Netdata in Alpine Linux 3.x:

```
# Install required packages
apk add alpine-sdk bash curl zlib-dev util-linux-dev libmnl-dev gcc make git autoconf automake pkgconfig python logrotate

# If you plan to run node.js Netdata plugins
apk add nodejs

# Download Netdata - the directory 'netdata' will be created
git clone https://github.com/netdata/netdata.git --depth=100
cd netdata

# Build it, install it, start it
./netdata-installer.sh

# Make Netdata start at boot
echo -e "#!/usr/bin/env bash\n/usr/sbin/netdata" >/etc/local.d/netdata.start
chmod 755 /etc/local.d/netdata.start

# Make Netdata stop at shutdown
echo -e "#!/usr/bin/env bash\nkillall netdata" >/etc/local.d/netdata.stop
chmod 755 /etc/local.d/netdata.stop

# Enable the local service to start automatically
rc-update add local
```


## Synology

The [64-bit static installer](#pre-built-static-binary-for-linux-64-bit) works fine if your Synology NAS uses the `amd64` architecture. That script will install Netdata into `/opt/netdata`, making future removal safe and simple.

When Netdata is first installed, it will run as `root`. This may or may not be acceptable for you, and since other installations run it as the `netdata` user, you might wish to do the same. This requires some extra work:

1. Create a group `netdata` via the Synology group interface. Give it no access to anything.
2. Create a user `netdata` via the Synology user interface. Give it no access to anything and a random password. Assign the user to the `netdata` group. Netdata will chuid to this user when running.
3. Change ownership of the following directories, as defined in [Netdata's security design](../../docs/netdata-security.md#security-design):

```
$ chown -R root:netdata /opt/netdata/usr/share/netdata
$ chown -R netdata:netdata /opt/netdata/var/lib/netdata /opt/netdata/var/cache/netdata
$ chown -R netdata:root /opt/netdata/var/log/netdata
```

Additionally, as of 2018/06/24, the Netdata installer doesn't recognize DSM as an operating system, so no init script is installed. You'll have to do this manually:

1. Add [this file](https://gist.github.com/oskapt/055d474d7bfef32c49469c1b53e8225f) as `/etc/rc.netdata`. Make it executable with `chmod 0755 /etc/rc.netdata`.
2. Edit `/etc/rc.local` and add a line calling `/etc/rc.netdata` to have it start on boot:

```
# Netdata startup
[ -x /etc/rc.netdata ] && /etc/rc.netdata start
```