<!--
title: "Install Netdata on Alpine 3.x"
custom_edit_url: https://github.com/netdata/netdata/edit/master/packaging/installer/methods/alpine.md
-->

# Install Netdata on Alpine 3.x

Execute these commands to install Netdata in Alpine Linux 3.x:

```sh
# install required packages
apk add alpine-sdk bash curl libuv-dev zlib-dev util-linux-dev libmnl-dev gcc make git autoconf automake pkgconfig python3 logrotate

# if you plan to run node.js Netdata plugins
apk add nodejs

# download Netdata - the directory 'netdata' will be created
git clone https://github.com/netdata/netdata.git --depth=100 --recursive
cd netdata

# build it, install it, start it
./netdata-installer.sh

# make Netdata start at boot and stop at shutdown
cat > /etc/init.d/netdata << EOF
#!/sbin/openrc-run

name="netdata"
command="/usr/sbin/$SVCNAME"

depend() {
        need net localmount
        after firewall
}
EOF
```

If you have installed Netdata in another directory, you have to change the content of the `command` variable in that script.

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fpackaging%2Finstaller%2Fmethods%2Falpine&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
