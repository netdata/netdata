<!--
title: "freeipmi.plugin"
custom_edit_url: "https://github.com/netdata/netdata/edit/master/collectors/freeipmi.plugin/README.md"
sidebar_label: "freeipmi.plugin"
learn_status: "Published"
learn_topic_type: "References"
learn_rel_path: "Integrations/Monitor/Devices"
-->

# freeipmi.plugin

Netdata has a [freeipmi](https://www.gnu.org/software/freeipmi/) plugin.

> FreeIPMI provides in-band and out-of-band IPMI software based on the IPMI v1.5/2.0 specification. The IPMI specification defines a set of interfaces for platform management and is implemented by a number vendors for system management. The features of IPMI that most users will be interested in are sensor monitoring, system event monitoring, power control, and serial-over-LAN (SOL).

## Installing the FreeIPMI plugin

When using our official DEB/RPM packages, the FreeIPMI plugin is included in a separate package named
`netdata-plugin-freeipmi` which needs to be manually installed using your system package manager. It is not
installed automatically due to the large number of dependencies it requires.

When using a static build of Netdata, the FreeIPMI plugin will be included and installed automatically, though
you will still need to have FreeIPMI installed on your system to be able to use the plugin.

When using a local build of Netdata, you need to ensure that the FreeIPMI development packages (typically called `libipmimonitoring-dev`, `libipmimonitoring-devel`, or `freeipmi-devel`) are installed when building Netdata.

### Special Considerations

Accessing IPMI requires root access, so the FreeIPMI plugin is automatically installed setuid root.

FreeIPMI does not work correctly on IBM POWER systems, thus Netdata’s FreeIPMI plugin is not usable on such systems.

If you have not previously used IPMI on your system, you will probably need to run the `ipmimonitoring` command as root to initiailze IPMI settings so that the Netdata plugin works correctly. It should return information about available seensors on the system.

In some distributions `libipmimonitoring.pc` is located in a non-standard directory, which
can cause building the plugin to fail when building Netdata from source. In that case you
should find the file and link it to the standard pkg-config directory. Usually, running `sudo ln -s
/usr/lib/$(uname -m)-linux-gnu/pkgconfig/libipmimonitoring.pc/libipmimonitoring.pc /usr/lib/pkgconfig/libipmimonitoring.pc`
resolves this issue.

## Netdata use

The plugin creates (up to) 8 charts, based on the information collected from IPMI:

1.  number of sensors by state
2.  number of events in SEL
3.  Temperatures CELSIUS
4.  Temperatures FAHRENHEIT
5.  Voltages
6.  Currents
7.  Power
8.  Fans

It also adds 2 alarms:

1.  Sensors in non-nominal state (i.e. warning and critical)
2.  SEL is non empty

![image](https://cloud.githubusercontent.com/assets/2662304/23674138/88926a20-037d-11e7-89c0-20e74ee10cd1.png)

The plugin does a speed test when it starts, to find out the duration needed by the IPMI processor to respond. Depending on the speed of your IPMI processor, charts may need several seconds to show up on the dashboard.

## `freeipmi.plugin` configuration

The plugin supports a few options. To see them, run:

```text
# /usr/libexec/netdata/plugins.d/freeipmi.plugin -h

 netdata freeipmi.plugin 1.8.0-546-g72ce5d6b_rolling
 Copyright (C) 2016-2017 Costa Tsaousis <costa@tsaousis.gr>
 Released under GNU General Public License v3 or later.
 All rights reserved.

 This program is a data collector plugin for netdata.

 Available command line options:

  SECONDS                 data collection frequency
                          minimum: 5

  debug                   enable verbose output
                          default: disabled

  sel
  no-sel                  enable/disable SEL collection
                          default: enabled

  hostname HOST
  username USER
  password PASS           connect to remote IPMI host
                          default: local IPMI processor

  noauthcodecheck         don't check the authentication codes returned

  driver-type IPMIDRIVER
                          Specify the driver type to use instead of doing an auto selection. 
                          The currently available outofband drivers are LAN and  LAN_2_0,
                          which  perform  IPMI  1.5  and  IPMI  2.0 respectively. 
                          The currently available inband drivers are KCS, SSIF, OPENIPMI and SUNBMC.

  sdr-cache-dir PATH      directory for SDR cache files
                          default: /tmp

  sensor-config-file FILE filename to read sensor configuration
                          default: system default

  ignore N1,N2,N3,...     sensor IDs to ignore
                          default: none

  -v
  -V
  version                 print version and exit

 Linux kernel module for IPMI is CPU hungry.
 On Linux run this to lower kipmiN CPU utilization:
 # echo 10 > /sys/module/ipmi_si/parameters/kipmid_max_busy_us

 or create: /etc/modprobe.d/ipmi.conf with these contents:
 options ipmi_si kipmid_max_busy_us=10

 For more information:
 https://github.com/netdata/netdata/tree/master/collectors/freeipmi.plugin
```

You can set these options in `/etc/netdata/netdata.conf` at this section:

```
[plugin:freeipmi]
	update every = 5
	command options = 
```

Append to `command options =` the settings you need. The minimum `update every` is 5 (enforced internally by the plugin). IPMI is slow and CPU hungry. So, once every 5 seconds is pretty acceptable.

## Ignoring specific sensors

Specific sensor IDs can be excluded from freeipmi tools by editing `/etc/freeipmi/freeipmi.conf` and setting the IDs to be ignored at `ipmi-sensors-exclude-record-ids`. **However this file is not used by `libipmimonitoring`** (the library used by Netdata's `freeipmi.plugin`).

So, `freeipmi.plugin` supports the option `ignore` that accepts a comma separated list of sensor IDs to ignore. To configure it, edit `/etc/netdata/netdata.conf` and set:

```
[plugin:freeipmi]
	command options = ignore 1,2,3,4,...
```

To find the IDs to ignore, run the command `ipmimonitoring`. The first column is the wanted ID:

```
ID  | Name             | Type                     | State    | Reading    | Units | Event
1   | Ambient Temp     | Temperature              | Nominal  | 26.00      | C     | 'OK'
2   | Altitude         | Other Units Based Sensor | Nominal  | 480.00     | ft    | 'OK'
3   | Avg Power        | Current                  | Nominal  | 100.00     | W     | 'OK'
4   | Planar 3.3V      | Voltage                  | Nominal  | 3.29       | V     | 'OK'
5   | Planar 5V        | Voltage                  | Nominal  | 4.90       | V     | 'OK'
6   | Planar 12V       | Voltage                  | Nominal  | 11.99      | V     | 'OK'
7   | Planar VBAT      | Voltage                  | Nominal  | 2.95       | V     | 'OK'
8   | Fan 1A Tach      | Fan                      | Nominal  | 3132.00    | RPM   | 'OK'
9   | Fan 1B Tach      | Fan                      | Nominal  | 2150.00    | RPM   | 'OK'
10  | Fan 2A Tach      | Fan                      | Nominal  | 2494.00    | RPM   | 'OK'
11  | Fan 2B Tach      | Fan                      | Nominal  | 1825.00    | RPM   | 'OK'
12  | Fan 3A Tach      | Fan                      | Nominal  | 3538.00    | RPM   | 'OK'
13  | Fan 3B Tach      | Fan                      | Nominal  | 2625.00    | RPM   | 'OK'
14  | Fan 1            | Entity Presence          | Nominal  | N/A        | N/A   | 'Entity Present'
15  | Fan 2            | Entity Presence          | Nominal  | N/A        | N/A   | 'Entity Present'
...
```

## Debugging

You can run the plugin by hand:

```sh
# become user netdata
sudo su -s /bin/sh netdata

# run the plugin in debug mode
/usr/libexec/netdata/plugins.d/freeipmi.plugin 5 debug
```

You will get verbose output on what the plugin does.

## kipmi0 CPU usage

There have been reports that kipmi is showing increased CPU when the IPMI is queried. To lower the CPU consumption of
the system you can issue this command:

```sh
echo 10 > /sys/module/ipmi_si/parameters/kipmid_max_busy_us
```

You can also permanently set the above setting by creating the file `/etc/modprobe.d/ipmi.conf` with this content:

```sh
# prevent kipmi from consuming 100% CPU
options ipmi_si kipmid_max_busy_us=10
```

This instructs the kernel IPMI module to pause for a tick between checking IPMI. Querying IPMI will be a lot slower now (e.g. several seconds for IPMI to respond), but `kipmi` will not use any noticeable CPU. You can also use a higher number (this is the number of microseconds to poll IPMI for a response, before waiting for a tick).

If you need to disable IPMI for Netdata, edit `/etc/netdata/netdata.conf` and set:

```
[plugins]
    freeipmi = no
```
