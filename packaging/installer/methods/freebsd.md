<!--
title: "Install Netdata on FreeBSD"
description: "Install Netdata on FreeBSD to monitor the health and performance of bare metal or VMs with thousands of real-time, per-second metrics."
custom_edit_url: https://github.com/netdata/netdata/edit/master/packaging/installer/methods/freebsd.md
-->

# Install Netdata on FreeBSD

> 💡 This document is maintained by Netdata's community, and may not be completely up-to-date. Please double-check the
> details of the installation process, such as version numbers for downloadable packages, before proceeding.
>
> You can help improve this document by [submitting a
> PR](https://github.com/netdata/netdata/edit/master/packaging/installer/methods/freebsd.md) with your recommended
> improvements or changes. Thank you!

## Install dependencies

This step needs root privileges.

```sh
pkg install bash e2fsprogs-libuuid git curl autoconf automake pkgconf pidof liblz4 libuv json-c cmake gmake
```

Please respond in the affirmative for any relevant prompts during the installation process. 

## Install Netdata

The simplest method is to use the single line [kickstart script](https://learn.netdata.cloud/docs/agent/packaging/installer/methods/kickstart)

If you have a Netdata cloud account then clicking on the **Connect Nodes** button will generate the kickstart command you should use. Use the command from the "Linux" tab, it should look something like this:

```sh
wget -O /tmp/netdata-kickstart.sh https://my-netdata.io/kickstart.sh && sh /tmp/netdata-kickstart.sh --claim-token <CLAIM_TOKEN> --claim-url https://app.netdata.cloud
```
Please respond in the affirmative for any relevant prompts during the installation process. 

Once the installation is completed, you should be able to start monitoring the FreeBSD server using Netdata. 

![image](https://user-images.githubusercontent.com/24860547/202489210-3c5a3346-8f53-4b7b-9832-f9383b34d864.png)

Netdata can also be installed via [FreeBSD ports](https://www.freshports.org/net-mgmt/netdata).

## Manual installation

If you would prefer to manually install Netdata, the following steps can help you do this.

Download Netdata:

```sh
fetch https://github.com/netdata/netdata/releases/download/v1.36.1/netdata-v1.36.1.tar.gz
```

> ⚠️ Verify the latest version by either navigating to [Netdata's latest
> release](https://github.com/netdata/netdata/releases/latest) or using `curl`:
>
> ```bash
> basename $(curl -Ls -o /dev/null -w %{url_effective} https://github.com/netdata/netdata/releases/latest)
> ```

Unzip the downloaded file:

```sh
gunzip netdata*.tar.gz && tar xf netdata*.tar && rm -rf netdata*.tar
```

Install Netdata in `/opt/netdata`. If you want to enable automatic updates, add `--auto-update` or `-u` to install `netdata-updater` in `cron` (**need root permission**):

```sh
cd netdata-v* && ./netdata-installer.sh --install /opt && cp /opt/netdata/usr/sbin/netdata-claim.sh /usr/sbin/
```

You also need to enable the `netdata` service in `/etc/rc.conf`:

```sh
sysrc netdata_enable="YES"
```

Finally, and very importantly, update Netdata using the script provided by the Netdata team (**need root permission**):

```sh
cd /opt/netdata/usr/libexec/netdata/ && ./netdata-updater.sh
```

You can now access the Netdata dashboard by navigating to `http://NODE:19999`, replacing `NODE` with the IP address or hostname of your system.

![image](https://user-images.githubusercontent.com/2662304/48304090-fd384080-e51b-11e8-80ae-eecb03118dda.png)

Starting with v1.30, Netdata collects anonymous usage information by default and sends it to a self hosted PostHog instance within the Netdata infrastructure. To read
more about the information collected and how to opt-out, check the [anonymous statistics
page](/docs/anonymous-statistics.md).

## Updating the Agent on FreeBSD
If you have not passed the `--auto-update` or `-u` parameter for the installer to enable automatic updating, repeat the last step to update Netdata whenever a new version becomes available. 
The `netdata-updater.sh` script will update your Agent.

## Optional parameters to alter your installation
| parameters | Description |
|:-----:|-----------|
|`--install <path>`| Install netdata in `<path>.` Ex: `--install /opt` will put netdata in `/opt/netdata`|
| `--dont-start-it` | Do not (re)start netdata after installation|
| `--dont-wait` | Run installation in non-interactive mode|
| `--auto-update` or `-u` | Install netdata-updater in cron to update netdata automatically once per day|
| `--stable-channel` | Use packages from GitHub release pages instead of GCS (nightly updates). This results in less frequent updates|
| `--nightly-channel` | Use most recent nightly updates instead of GitHub releases. This results in more frequent updates|
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
| `--disable-exporting-kinesis` | Disable AWS Kinesis exporting connector. Default: enable it when libaws_cpp_sdk_kinesis and libraries (it depends on are available)|
| `--enable-exporting-prometheus-remote-write` | Enable Prometheus remote write exporting connector. Default: enable it when libprotobuf and libsnappy are available|
| `--disable-exporting-prometheus-remote-write` | Disable Prometheus remote write exporting connector. Default: enable it when libprotobuf and libsnappy are available|
| `--enable-exporting-mongodb` | Enable MongoDB exporting connector. Default: enable it when libmongoc is available|
| `--disable-exporting-mongodb` | Disable MongoDB exporting connector|
| `--enable-lto` | Enable Link-Time-Optimization. Default: enabled|
| `--disable-lto` | Disable Link-Time-Optimization. Default: enabled|
| `--disable-x86-sse` | Disable SSE instructions. By default SSE optimizations are enabled|
| `--zlib-is-really-here` or `--libs-are-really-here` | If you get errors about missing zlib or libuuid but you know it is available, you might have a broken pkg-config. Use this option to proceed without checking pkg-config|
|`--disable-telemetry` | Use this flag to opt-out from our anonymous telemetry program. (DISABLE_TELEMETRY=1)|


