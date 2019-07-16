# Other installation methods

The Netdata team works hard to make Netdata installable on as many systems as possible. This page contains installation instructions for some less common, or less supported, operating systems and machines.

If you're installing Linux on a Linux system, we recommend you try our [one-line automatic installation](README.md#one-line-installation) or [binary releases](README.md#binary-packages) first. If those don't work, you can try the [pre-built static binary](#pre-built-static-binary-for-linux-64-bit) or the [manual installation](MANUAL-INSTALLATION.md).

- [Pre-built static binary](#pre-built-static-binary-for-linux-64-bit)
- [macOS](#macos)
- [FreeBSD](#freebsd)
- [pfSense](#pfsense)
- [FreeNAS](#freenas)
- [Alpine 3.x](#alpine-3-x)
- [Synology](#synology)


## Pre-built static binary for Linux 64-bit
![](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.requests_per_url&options=unaligned&dimensions=kickstart64&group=sum&after=-3600&label=last+hour&units=installations&value_color=orange&precision=0) ![](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.requests_per_url&options=unaligned&dimensions=kickstart64&group=sum&after=-86400&label=today&units=installations&precision=0)

You can install a pre-compiled static binary of Netdata on any Intel/AMD 64bit Linux system (even those that don't have a package manager, like CoreOS, CirrOS, busybox systems, etc). You can also use these packages on systems with broken or unsupported package managers.

To install Netdata from a binary package on any Linux distro and any kernel version on **Intel/AMD 64bit** systems, and get **automatic, nightly** updates, run the following:

```bash
$ bash <(curl -Ss https://my-netdata.io/kickstart-static64.sh)
```

!!! note
    Do not use `sudo` for this installer—it will escalate privileges itself if needed.

    To learn more about the pros and cons of using *nightly* vs. *stable* releases, see our [notice about the two options](README.md#nightly-vs-stable-releases).

    If your system does not have `bash` installed, open the `More information and advanced uses of the kickstart-static64.sh script` dropdown for instructions to run the installer without `bash`.

<details markdown="1"><summary>More information and advanced uses of the `kickstart-static64.sh` script</summary>

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

If the one-line installation script fails—for example, if you do not have `bash` installed—you can use the following commands to run 
If your shell fails to handle the above one liner, do this:

```bash
# download the script with curl
curl https://my-netdata.io/kickstart-static64.sh >/tmp/kickstart-static64.sh

# or, download the script with wget
wget -O /tmp/kickstart-static64.sh https://my-netdata.io/kickstart-static64.sh

# run the downloaded script (any sh is fine, no need for bash)
sh /tmp/kickstart-static64.sh
```

- The static binary files are kept in repo [binary-packages](https://github.com/netdata/binary-packages). You can download any of the `.run` files, and run it. These files are self-extracting shell scripts built with [makeself](https://github.com/megastep/makeself).
- The target system does **not** need to have bash installed.
- The same files can be used for updates too.
- For QA purposes, this installation method lets us know if it succeed or failed.

</details>

Once you have installed Netdata, see our [getting started guide](../../docs/GettingStarted.md).

---

## macOS

Netdata on macOS still has limited charts, but external plugins do work.

You can either install Netdata with [Homebrew](https://brew.sh/)

```sh
brew install netdata
```

or from source:

```sh
# install Xcode Command Line Tools
xcode-select --install
```
click `Install` in the software update popup window, then
```sh
# install HomeBrew package manager
/usr/bin/ruby -e "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/master/install)"

# install required packages
brew install ossp-uuid autoconf automake pkg-config

# download Netdata
git clone https://github.com/netdata/netdata.git --depth=100

# install Netdata in /usr/local/netdata
cd netdata
sudo ./netdata-installer.sh --install /usr/local
```

The installer will also install a startup plist to start Netdata when your Mac boots.


## FreeBSD

You can install Netdata from ports or packages collection.

This is how to install the latest Netdata version from sources on FreeBSD:

```sh
# install required packages
pkg install bash e2fsprogs-libuuid git curl autoconf automake pkgconf pidof

# download Netdata
git clone https://github.com/netdata/netdata.git --depth=100

# install Netdata in /opt/netdata
cd netdata
./netdata-installer.sh --install /opt
```

## pfSense

To install Netdata on pfSense, run the following commands (within a shell or under Diagnostics/Command Prompt within the pfSense web interface).

Change platform (i386/amd64, etc) and FreeBSD versions (10/11, etc) according to your environment and change Netdata version (1.10.0 in example) according to latest version present within the FreeSBD repository:-

Note first three packages are downloaded from the pfSense repository for maintaining compatibility with pfSense, Netdata is downloaded from the FreeBSD repository.
```
pkg install pkgconf
pkg install bash
pkg install e2fsprogs-libuuid
pkg add http://pkg.freebsd.org/FreeBSD:11:amd64/latest/All/python36-3.6.8_2.txz
pkg add http://pkg.freebsd.org/FreeBSD:11:amd64/latest/All/netdata-1.13.0.txz
```
To start Netdata manually run `service netdata onestart`

To start Netdata automatically at each boot add `service netdata onestart` as a Shellcmd within the pfSense web interface (under **Services/Shellcmd**, which you need to install beforehand under **System/Package Manager/Available Packages**).
Shellcmd Type should be set to `Shellcmd`.
![](https://i.imgur.com/wcKiPe1.png)
Alternatively more information can be found in https://doc.pfsense.org/index.php/Installing_FreeBSD_Packages, for achieving the same via the command line and scripts.

If you experience an issue with `/usr/bin/install` absense on pfSense 2.3 or earlier, update pfSense or use workaround from [https://redmine.pfsense.org/issues/6643](https://redmine.pfsense.org/issues/6643)  

**Note:** In pfSense, the Netdata configuration files are located under `/usr/local/etc/netdata`


## FreeNAS

On FreeNAS-Corral-RELEASE (>=10.0.3), Netdata is pre-installed.

To use Netdata, the service will need to be enabled and started from the FreeNAS **[CLI](https://github.com/freenas/cli)**.

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
# install required packages
apk add alpine-sdk bash curl zlib-dev util-linux-dev libmnl-dev gcc make git autoconf automake pkgconfig python logrotate

# if you plan to run node.js Netdata plugins
apk add nodejs

# download Netdata - the directory 'netdata' will be created
git clone https://github.com/netdata/netdata.git --depth=100
cd netdata

# build it, install it, start it
./netdata-installer.sh

# make Netdata start at boot
echo -e "#!/usr/bin/env bash\n/usr/sbin/netdata" >/etc/local.d/netdata.start
chmod 755 /etc/local.d/netdata.start

# make Netdata stop at shutdown
echo -e "#!/usr/bin/env bash\nkillall netdata" >/etc/local.d/netdata.stop
chmod 755 /etc/local.d/netdata.stop

# enable the local service to start automatically
rc-update add local
```


## Synology

The documentation previously recommended installing the Debian Chroot package from the Synology community package sources and then running Netdata from within the chroot. This does not work, as the chroot environment does not have access to `/proc`, and therefore exposes very few metrics to Netdata. Additionally, [this issue](https://github.com/SynoCommunity/spksrc/issues/2758), still open as of 2018/06/24, indicates that the Debian Chroot package is not suitable for DSM versions greater than version 5 and may corrupt system libraries and render the NAS unable to boot.

The good news is that the 64-bit static installer works fine if your NAS is one that uses the amd64 architecture. It will install the content into `/opt/netdata`, making future removal safe and simple.

When Netdata is first installed, it will run as _root_. This may or may not be acceptable for you, and since other installations run it as the _netdata_ user, you might wish to do the same. This requires some extra work:

1. Creat a group `netdata` via the Synology group interface. Give it no access to anything.
2. Create a user `netdata` via the Synology user interface. Give it no access to anything and a random password. Assign the user to the `netdata` group. Netdata will chuid to this user when running.
3. Change ownership of the following directories, as defined in [Netdata Security](../../docs/netdata-security.md#security-design):

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