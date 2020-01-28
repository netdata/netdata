# Install Netdata on Alpine 3.x

Execute these commands to install Netdata in Alpine Linux 3.x:

```sh
# install required packages
apk add alpine-sdk bash curl libuv-dev zlib-dev util-linux-dev libmnl-dev gcc make git autoconf automake pkgconfig python logrotate

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
