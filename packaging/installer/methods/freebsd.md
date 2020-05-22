<!--
---
title: "Install Netdata on FreeBSD"
custom_edit_url: https://github.com/netdata/netdata/edit/master/packaging/installer/methods/freebsd.md
---
-->
![image](https://github.com/thoggs/netdata/blob/master/packaging/installer/methods/freebsd-logo.png)

# Install Netdata on FreeBSD

# Netdata ![](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.requests_per_url&options=unaligned&dimensions=kickstart&group=sum&after=-3600&label=last+hour&units=installations&value_color=orange&precision=0) ![](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.requests_per_url&options=unaligned&dimensions=kickstart&group=sum&after=-86400&label=today&units=installations&precision=0)

## Install latest version
This is how to install the latest Netdata version on FreeBSD:

Install required packages (need root permission)

```sh
# pkg install bash e2fsprogs-libuuid git curl autoconf automake pkgconf pidof Judy liblz4 libuv json-c cmake
```

Download Netdata

```sh
# fetch https://github.com/netdata/netdata/releases/download/v1.22.1/netdata-v1.22.1.tar.gz
```

Unzip the downloaded file so you can install

```sh
# gunzip netdata*.tar.gz && tar xf netdata*.tar && rm -rf netdata*.tar
```

Install Netdata in /opt/netdata (need root permission)

```sh
# cd netdata-v* && ./netdata-installer.sh --install /opt
```

Now we will include the flag that will make Netdata boot with FreeBSD, whenever you turn on or restart your computer (need root permission):

```sh
# sysrc netdata_enable="YES"
```

Finally, and very importantly, update Netdata using the script provided by the Netdata team (need root permission):

```sh
# cd /opt/netdata/usr/libexec/netdata/ && ./netdata-updater.sh
```

# Important
## Updating the Agent on FreeBSD
Repeat the last step to update Netdata whenever a new version becomes available. 
The `netdata-updater.sh` script will update your Agent. 
For now, this is the way that the Netdata team offers to update the Agent when there is a new version.

## Optional parameters to alter your installation
| parameters | Description |
|:-----:|-----------|
|`--install <path>`| Install netdata in <path>. Ex. --install /opt` will put netdata in /opt/netdata|
| `--dont-start-it` | Do not (re)start netdata after installation|
| `--dont-wait` | Run installation in non-interactive mode|
| `--auto-update` or `-u` | Install netdata-updater in cron to update netdata automatically once per day|
| `--stable-channel` | Use packages from GitHub release pages instead of GCS (nightly updates). This results in less frequent updates|
| `--nightly-channel` | Use most recent nightly udpates instead of GitHub releases. This results in more frequent updates|
| `--disable-go` | Disable installation of go.d.plugin|
| `--disable-ebpf` | Disable eBPF Kernel plugin (Default: enabled)|
| `--disable-cloud` | Disable all Netdata Cloud functionality|
| `--require-cloud` | Fail the install if it can't build Netdata Cloud support|
| `--enable-plugin-freeipmi` | Enable the FreeIPMI plugin. Default: enable it when libipmimonitoring is available|
| `--disable-plugin-freeipmi` | Enable the FreeIPMI plugin|
| `--disable-https` | Explicitly disable TLS support|
| `--disable-dbengine` | Explicitly disable DB engine support|
| `--enable-plugin-nfacct` | Enable nfacct plugin. Default: enable it when libmnl and libnetfilter_acct are available|
| `--disable-plugin-nfacct` | Disable nfacct plugin. Default: enable it when libmnl and libnetfilter_acct are available|
| `--enable-plugin-xenstat` | Enable the xenstat plugin. Default: enable it when libxenstat and libyajl are available|
| `--disable-plugin-xenstat` | Disable the xenstat plugin|
| `--enable-backend-kinesis` | Enable AWS Kinesis backend. Default: enable it when libaws_cpp_sdk_kinesis and libraries (it depends on are available)|                           
| `--disable-backend-kinesis` | Disable AWS Kinesis backend. Default: enable it when libaws_cpp_sdk_kinesis and libraries (it depends on are available)|
| `--enable-backend-prometheus-remote-write` | Enable Prometheus remote write backend. Default: enable it when libprotobuf and libsnappy are available|
| `--disable-backend-prometheus-remote-write` | Disable Prometheus remote write backend. Default: enable it when libprotobuf and libsnappy are available|
| `--enable-backend-mongodb` | Enable MongoDB backend. Default: enable it when libmongoc is available|
| `--disable-backend-mongodb` | Disable MongoDB backend|
| `--enable-lto` | Enable Link-Time-Optimization. Default: enabled|
| `--disable-lto` | Disable Link-Time-Optimization. Default: enabled|
| `--disable-x86-sse` | Disable SSE instructions. By default SSE optimizations are enabled|
| `--zlib-is-really-here` or `--libs-are-really-here` | If you get errors about missing zlib or libuuid but you know it is available, you might have a broken pkg-config. Use this option to proceed without checking pkg-config|
| `--disable-telemetry` | Use this flag to opt-out from our anonymous telemetry progam. (DO_NOT_TRACK=1)|
