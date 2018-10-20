netdata has a [freeipmi](https://www.gnu.org/software/freeipmi/) plugin.

> FreeIPMI provides in-band and out-of-band IPMI software based on the IPMI v1.5/2.0 specification. The IPMI specification defines a set of interfaces for platform management and is implemented by a number vendors for system management. The features of IPMI that most users will be interested in are sensor monitoring, system event monitoring, power control, and serial-over-LAN (SOL).

## compile `freeipmi.plugin`

1. install `libipmimonitoring-dev` or `libipmimonitoring-devel` (`freeipmi-devel` on RHEL based OS) using the package manager of your system.

2. re-install netdata from source. The installer will detect that the required libraries are now available and will also build `freeipmi.plugin`.

Keep in mind IPMI requires root access, so the plugin is setuid to root.

If you just installed the required IPMI tools, please run at least once the command `ipmimonitoring` and verify it returns sensors information. This command initialises IPMI configuration, so that the netdata plugin will be able to work.

## netdata use

The plugin creates (up to) 8 charts, based on the information collected from IPMI:

1. number of sensors by state
2. number of events in SEL
3. Temperatures CELCIUS
4. Temperatures FAHRENHEIT
5. Voltages
6. Currents
7. Power
8. Fans


It also adds 2 alarms:

1. Sensors in non-nominal state (i.e. warning and critical)
2. SEL is non empty

![image](https://cloud.githubusercontent.com/assets/2662304/23674138/88926a20-037d-11e7-89c0-20e74ee10cd1.png)

The plugin does a speed test when it starts, to find out the duration needed by the IPMI processor to respond. Depending on the speed of your IPMI processor, charts may need several seconds to show up on the dashboard.

## `freeipmi.plugin` configuration

The plugin supports a few options. To see them, run:

```sh
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
 https://github.com/ktsaou/netdata/tree/master/plugins/freeipmi.plugin

```

You can set these options in `/etc/netdata/netdata.conf` at this section:

```
[plugin:freeipmi]
	update every = 5
	command options = 
```

Append to `command options = ` the settings you need. The minimum `update every` is 5 (enforced internally by the plugin). IPMI is slow and CPU hungry. So, once every 5 seconds is pretty acceptable.

## ignoring specific sensors

Specific sensor IDs can be excluded from freeipmi tools by editing `/etc/freeipmi/freeipmi.conf` and setting the IDs to be ignored at `ipmi-sensors-exclude-record-ids`. **However this file is not used by `libipmimonitoring`** (the library used by netdata's `freeipmi.plugin`).

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


## debugging

You can run the plugin by hand:

```sh
# become user netdata
sudo su -s /bin/sh netdata

# run the plugin in debug mode
/usr/libexec/netdata/plugins.d/freeipmi.plugin 5 debug
```

You will get verbose output on what the plugin does.

## kipmi0 CPU usage

There have been reports that kipmi is showing increased CPU when the IPMI is queried.

[IBM has given a few explanations](http://www-01.ibm.com/support/docview.wss?uid=nas7d580df3d15874988862575fa0050f604).

Check also [this stackexchange post](http://unix.stackexchange.com/questions/74900/kipmi0-eating-up-to-99-8-cpu-on-centos-6-4).

To lower the CPU consumption of the system you can issue this command:

```sh
echo 10 > /sys/module/ipmi_si/parameters/kipmid_max_busy_us
```

You can also permanently set the above setting by creating the file `/etc/modprobe.d/ipmi.conf` with this content:

```sh
# prevent kipmi from consuming 100% CPU
options ipmi_si kipmid_max_busy_us=10
```

This instructs the kernel IPMI module to pause for a tick between checking IPMI. Querying IPMI will be a lot slower now (e.g. several seconds for IPMI to respond), but `kipmi` will not use any noticeable CPU. You can also use a higher number (this is the number of microseconds to poll IPMI for a response, before waiting for a tick).

If you need to disable IPMI for netdata, edit `/etc/netdata/netdata.conf` and set:

```
[plugins]
    freeipmi = no
```
