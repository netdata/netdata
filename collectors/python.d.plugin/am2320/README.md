<!--
title: "AM2320 sensor monitoring with netdata"
custom_edit_url: "https://github.com/netdata/netdata/edit/master/collectors/python.d.plugin/am2320/README.md"
sidebar_label: "AM2320"
learn_status: "Published"
learn_topic_type: "References"
learn_rel_path: "Integrations/Monitor/Remotes/Devices"
-->

# AM2320 sensor monitoring with netdata

Displays a graph of the temperature and humidity from a AM2320 sensor.

## Requirements
 - Adafruit Circuit Python AM2320 library
 - Adafruit AM2320 I2C sensor
 - Python 3 (Adafruit libraries are not Python 2.x compatible)
 

It produces the following charts:
1. **Temperature** 
2. **Humidity**

## Configuration

Edit the `python.d/am2320.conf` configuration file using `edit-config` from the Netdata [config
directory](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md), which is typically at `/etc/netdata`.

```bash
cd /etc/netdata   # Replace this path with your Netdata config directory, if different
sudo ./edit-config python.d/am2320.conf
```

Raspberry Pi Instructions:

Hardware install:
Connect the am2320 to the Raspberry Pi I2C pins

Raspberry Pi 3B/4 Pins:

- Board 3.3V (pin 1) to sensor VIN (pin 1)
- Board SDA (pin 3) to sensor SDA (pin 2)
- Board GND (pin 6) to sensor GND (pin 3)
- Board SCL (pin 5) to sensor SCL (pin 4)

You may also need to add two I2C pullup resistors if your board does not already have them. The Raspberry Pi does have internal pullup resistors but it doesn't hurt to add them anyway. You can use 2.2K - 10K but we will just use 10K. The resistors go from VDD to SCL and SDA each.

Software install:
- `sudo pip3 install adafruit-circuitpython-am2320`
- edit `/etc/netdata/netdata.conf`
- find `[plugin:python.d]`
- add  `command options = -ppython3`
- save the file.
- restart the netdata service.
- check the dashboard.

### Troubleshooting

To troubleshoot issues with the `am2320` module, run the `python.d.plugin` with the debug option enabled. The 
output will give you the output of the data collection job or error messages on why the collector isn't working.

First, navigate to your plugins directory, usually they are located under `/usr/libexec/netdata/plugins.d/`. If that's 
not the case on your system, open `netdata.conf` and look for the setting `plugins directory`. Once you're in the 
plugin's directory, switch to the `netdata` user.

```bash
cd /usr/libexec/netdata/plugins.d/
sudo su -s /bin/bash netdata
```

Now you can manually run the `am2320` module in debug mode:

```bash
./python.d.plugin am2320 debug trace
```

