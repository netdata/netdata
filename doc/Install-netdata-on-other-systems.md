# Install netdata on other systems

We are trying to collect all the information about netdata package maintainers at [issue 651](https://github.com/netdata/netdata/issues/651). So, please have a look there for ASUSTOR NAS, OpenWRT, ReadyNAS, etc.

## FreeBSD

You can install netdata from ports or packages collection.

This is how to install the latest netdata version from sources on FreeBSD:

```sh
# install required packages
pkg install bash e2fsprogs-libuuid git curl autoconf automake pkgconf pidof

# download netdata
git clone https://github.com/netdata/netdata.git --depth=1

# install netdata in /opt/netdata
cd netdata
./netdata-installer.sh --install /opt
```

## pfSense
To install netdata on pfSense run the following commands (within a shell or under Diagnostics/Command Prompt within the pfSense web interface).

Change platform (i386/amd64, etc) and FreeBSD versions (10/11, etc) according to your environment and change netdata version (1.10.0 in example) according to latest version present within the FreeSBD repository:-

Note first three packages are downloaded from the pfSense repository for maintaining compatibility with pfSense, netdata is downloaded from the FreeBSD repository.
```
pkg install pkgconf
pkg install bash
pkg install e2fsprogs-libuuid
pkg add http://pkg.freebsd.org/FreeBSD:11:amd64/latest/All/netdata-1.10.0.txz
```
To start netdata manually run `service netdata onestart`

To start netdata automatically at each boot add `service netdata start` as a Shellcmd within the pfSense web interface (under **Services/Shellcmd**, which you need to install beforehand under **System/Package Manager/Available Packages**).
Shellcmd Type should be set to `Shellcmd`. 
![](https://user-images.githubusercontent.com/36808164/36930790-4db3aa84-1f0d-11e8-8752-cdc08bb7207c.png)
Alternatively more information can be found in https://doc.pfsense.org/index.php/Installing_FreeBSD_Packages, for achieving the same via the command line and scripts.  

If you experience an issue with `/usr/bin/install` absense on pfSense 2.3 or earlier, update pfSense or use workaround from [https://redmine.pfsense.org/issues/6643](https://redmine.pfsense.org/issues/6643)

## FreeNAS
On FreeNAS-Corral-RELEASE (>=10.0.3), netdata is pre-installed. 

To use netdata, the service will need to be enabled and started from the FreeNAS **[CLI](https://github.com/freenas/cli)**.

To enable the netdata service:
```
service netdata config set enable=true
```

To start the netdata service:
```
service netdata start
```

## macOS

netdata on macOS still has limited charts, but external plugins do work.

You can either install netdata with [Homebrew](https://brew.sh/)

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

# download netdata
git clone https://github.com/netdata/netdata.git --depth=1

# install netdata in /usr/local/netdata
cd netdata
sudo ./netdata-installer.sh --install /usr/local
```

The installer will also install a startup plist to start netdata when your Mac boots.

## Alpine 3.x

Execute these commands to install netdata in Alpine Linux 3.x:

```
# install required packages
apk add alpine-sdk bash curl zlib-dev util-linux-dev libmnl-dev gcc make git autoconf automake pkgconfig python logrotate

# if you plan to run node.js netdata plugins
apk add nodejs

# download netdata - the directory 'netdata' will be created
git clone https://github.com/netdata/netdata.git --depth=1
cd netdata


# build it, install it, start it
./netdata-installer.sh


# make netdata start at boot
echo -e "#!/usr/bin/env bash\n/usr/sbin/netdata" >/etc/local.d/netdata.start
chmod 755 /etc/local.d/netdata.start

# make netdata stop at shutdown
echo -e "#!/usr/bin/env bash\nkillall netdata" >/etc/local.d/netdata.stop
chmod 755 /etc/local.d/netdata.stop

# enable the local service to start automatically
rc-update add local
```

## Synology

The documentation previously recommended installing the Debian Chroot package from the Synology community package sources and then running netdata from within the chroot. This does not work, as the chroot environment does not have access to `/proc`, and therefore exposes very few metrics to netdata. Additionally, [this issue](https://github.com/SynoCommunity/spksrc/issues/2758), still open as of 2018/06/24, indicates that the Debian Chroot package is not suitable for DSM versions greater than version 5 and may corrupt system libraries and render the NAS unable to boot. 

The good news is that the 64-bit static installer works fine if your NAS is one that uses the amd64 architecture. It will install the content into `/opt/netdata`, making future removal safe and simple.

### Additional Work

When netdata is first installed, it will run as _root_. This may or may not be acceptable for you, and since other installations run it as the _netdata_ user, you might wish to do the same. This requires some extra work:

1. Creat a group `netdata` via the Synology group interface. Give it no access to anything.
2. Create a user `netdata` via the Synology user interface. Give it no access to anything and a random password. Assign the user to the `netdata` group. Netdata will chuid to this user when running.
3. Change ownership of the following directories, as defined in [Netdata Security](https://github.com/netdata/netdata/wiki/netdata-security):

```
$ chown -R root:netdata /opt/netdata/usr/share/netdata
$ chown -R netdata:netdata /opt/netdata/var/lib/netdata /opt/netdata/var/cache/netdata
$ chown -R netdata:root /opt/netdata/var/log/netdata
```

Additionally, as of 2018/06/24, the netdata installer doesn't recognize DSM as an operating system, so no init script is installed. You'll have to do this manually:

1. Add [this file](https://gist.github.com/oskapt/055d474d7bfef32c49469c1b53e8225f) as `/etc/rc.netdata`. Make it executable with `chmod 0755 /etc/rc.netdata`.
2. Edit `/etc/rc.local` and add a line calling `/etc/rc.netdata` to have it start on boot:

```
# Netdata startup
[ -x /etc/rc.netdata ] && /etc/rc.netdata start
```
