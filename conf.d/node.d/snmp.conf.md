# SNMP Data Collector

Using this collector, netdata can collect data from any SNMP device.

This collector supports:

- any number of SNMP devices
- each SNMP device can be used to collect data for any number of charts
- each chart may have any number of dimensions
- each SNMP device may have a different update frequency
- each SNMP device will accept one or more batches to report values (you can set `max_request_size` per SNMP server, to control the size of batches).

The source code of the plugin is [here](https://github.com/netdata/netdata/blob/master/node.d/snmp.node.js).

## Configuration

You will need to create the file `/etc/netdata/node.d/snmp.conf` with data like the following.

In this example:

 - the SNMP device is `10.11.12.8`.
 - the SNMP community is `public`.
 - we will update the values every 10 seconds (`update_every: 10` under the server `10.11.12.8`).
 - we define 2 charts `snmp_switch.bandwidth_port1` and `snmp_switch.bandwidth_port2`, each having 2 dimensions: `in` and `out`.

```js
{
    "enable_autodetect": false,
    "update_every": 5,
    "max_request_size": 100,
    "servers": [
        {
            "hostname": "10.11.12.8",
            "community": "public",
            "update_every": 10,
            "max_request_size": 50,
            "options": { "timeout": 10000 },
            "charts": {
                "snmp_switch.bandwidth_port1": {
                    "title": "Switch Bandwidth for port 1",
                    "units": "kilobits/s",
                    "type": "area",
                    "priority": 1,
                    "family": "ports",
                    "dimensions": {
                        "in": {
                            "oid": "1.3.6.1.2.1.2.2.1.10.1",
                            "algorithm": "incremental",
                            "multiplier": 8,
                            "divisor": 1024,
                            "offset": 0
                        },
                        "out": {
                            "oid": "1.3.6.1.2.1.2.2.1.16.1",
                            "algorithm": "incremental",
                            "multiplier": -8,
                            "divisor": 1024,
                            "offset": 0
                        }
                    }
                },
                "snmp_switch.bandwidth_port2": {
                    "title": "Switch Bandwidth for port 2",
                    "units": "kilobits/s",
                    "type": "area",
                    "priority": 1,
                    "family": "ports",
                    "dimensions": {
                        "in": {
                            "oid": "1.3.6.1.2.1.2.2.1.10.2",
                            "algorithm": "incremental",
                            "multiplier": 8,
                            "divisor": 1024,
                            "offset": 0
                        },
                        "out": {
                            "oid": "1.3.6.1.2.1.2.2.1.16.2",
                            "algorithm": "incremental",
                            "multiplier": -8,
                            "divisor": 1024,
                            "offset": 0
                        }
                    }
                }
            }
        }
    ]
}
```

`update_every` is the update frequency for each server, in seconds.

`max_request_size` limits the maximum number of OIDs that will be requested in a single call. The default is 50. Lower this number of you get `TooBig` errors in netdata error.log.

`family` sets the name of the submenu of the dashboard each chart will appear under.

If you need to define many charts using incremental OIDs, you can use something like this:

This is like the previous, but the option `multiply_range` given, will multiply the current chart from `1` to `24` inclusive, producing 24 charts in total for the 24 ports of the switch `10.11.12.8`.

Each of the 24 new charts will have its id (1-24) appended at:

1. its chart unique id, i.e. `snmp_switch.bandwidth_port1` to `snmp_switch.bandwidth_port24`
2. its `title`, i.e. `Switch Bandwidth for port 1` to `Switch Bandwidth for port 24`
3. its `oid` (for all dimensions), i.e. dimension `in` will be `1.3.6.1.2.1.2.2.1.10.1` to `1.3.6.1.2.1.2.2.1.10.24`
3. its priority (which will be incremented for each chart so that the charts will appear on the dashboard in this order)

```js
{
    "enable_autodetect": false,
    "update_every": 10,
    "servers": [
        {
            "hostname": "10.11.12.8",
            "community": "public",
            "update_every": 10,
            "options": { "timeout": 20000 },
            "charts": {
                "snmp_switch.bandwidth_port": {
                    "title": "Switch Bandwidth for port ",
                    "units": "kilobits/s",
                    "type": "area",
                    "priority": 1,
                    "family": "ports",
                    "multiply_range": [ 1, 24 ],
                    "dimensions": {
                        "in": {
                            "oid": "1.3.6.1.2.1.2.2.1.10.",
                            "algorithm": "incremental",
                            "multiplier": 8,
                            "divisor": 1024,
                            "offset": 0
                        },
                        "out": {
                            "oid": "1.3.6.1.2.1.2.2.1.16.",
                            "algorithm": "incremental",
                            "multiplier": -8,
                            "divisor": 1024,
                            "offset": 0
                        }
                    }
                }
            }
        }
    ]
}
```

The `options` given for each server, are:

 - `timeout`, the time to wait for the SNMP device to respond. The default is 5000 ms.
 - `version`, the SNMP version to use. `0` is Version 1, `1` is Version 2c. The default is Version 1 (`0`).
 - `transport`, the default is `udp4`.
 - `port`, the port of the SNMP device to connect to. The default is `161`.
 - `retries`, the number of attempts to make to fetch the data. The default is `1`.

## Retrieving names from snmp

You can append a value retrieved from SNMP to the title, by adding `titleoid` to the chart.

You can set a dimension name to a value retrieved from SNMP, by adding `oidname` to the dimension.

Both of the above will participate in `multiply_range`.


## Testing the configuration

To test it, you can run:

```sh
/usr/libexec/netdata/plugins.d/node.d.plugin 1 snmp
```

The above will run it on your console and you will be able to see what netdata sees, but also errors. You can get a very detailed output by appending `debug` to the command line.

If it works, restart netdata to activate the snmp collector and refresh the dashboard (if your SNMP device responds with a delay, you may need to refresh the dashboard in a few seconds).

## Data collection speed

Keep in mind that many SNMP switches are routers are very slow. They may not be able to report values per second. If you run `node.d.plugin` in `debug` mode, it will report the time it took for the SNMP device to respond. My switch, for example, needs 7-8 seconds to respond for the traffic on 24 ports (48 OIDs, in/out).

Also, if you use many SNMP clients on the same SNMP device at the same time, values may be skipped. This is a problem of the SNMP device, not this collector.

## Finding OIDs

Use `snmpwalk`, like this:

```sh
snmpwalk -t 20 -v 1 -O fn -c public 10.11.12.8
```

- `-t 20` is the timeout in seconds
- `-v 1` is the SNMP version
- `-O fn` will display full OIDs in numeric format (you may want to run it also without this option to see human readable output of OIDs)
- `-c public` is the SNMP community
- `10.11.12.8` is the SNMP device

Keep in mind that `snmpwalk` outputs the OIDs with a dot in front them. You should remove this dot when adding OIDs to the configuration file of this collector.

## Example: Linksys SRW2024P

This is what I use for my Linksys SRW2024P. It creates:

1. A chart for power consumption (it is a PoE switch)
2. Two charts for packets received (total packets received and packets received with errors)
3. One chart for packets output
4. 24 charts, one for each port of the switch. It also appends the port names, as defined at the switch, to the chart titles.

This switch also reports various other metrics, like snmp, packets per port, etc. Unfortunately it does not report CPU utilization or backplane utilization.

This switch has a very slow SNMP processors. To respond, it needs about 8 seconds, so I have set the refresh frequency (`update_every`) to 15 seconds.

```js
{
    "enable_autodetect": false,
    "update_every": 5,
    "servers": [
    {
        "hostname": "10.11.12.8",
        "community": "public",
        "update_every": 15,
        "options": { "timeout": 20000, "version": 1 },
        "charts": {
            "snmp_switch.power": {
                "title": "Switch Power Supply",
                "units": "watts",
                "type": "line",
                "priority": 10,
                "family": "power",
                "dimensions": {
                    "supply": {
                        "oid": ".1.3.6.1.2.1.105.1.3.1.1.2.1",
                        "algorithm": "absolute",
                        "multiplier": 1,
                        "divisor": 1,
                        "offset": 0
                    },
                    "used": {
                        "oid": ".1.3.6.1.2.1.105.1.3.1.1.4.1",
                        "algorithm": "absolute",
                        "multiplier": 1,
                        "divisor": 1,
                        "offset": 0
                    }
                }
            }
            , "snmp_switch.input": {
                "title": "Switch Packets Input",
                "units": "packets/s",
                "type": "area",
                "priority": 20,
                "family": "IP",
                "dimensions": {
                    "receives": {
                        "oid": ".1.3.6.1.2.1.4.3.0",
                        "algorithm": "incremental",
                        "multiplier": 1,
                        "divisor": 1,
                        "offset": 0
                    }
                    , "discards": {
                        "oid": ".1.3.6.1.2.1.4.8.0",
                        "algorithm": "incremental",
                        "multiplier": 1,
                        "divisor": 1,
                        "offset": 0
                    }
                }
            }
            , "snmp_switch.input_errors": {
                "title": "Switch Received Packets with Errors",
                "units": "packets/s",
                "type": "line",
                "priority": 30,
                "family": "IP",
                "dimensions": {
                    "bad_header": {
                        "oid": ".1.3.6.1.2.1.4.4.0",
                        "algorithm": "incremental",
                        "multiplier": 1,
                        "divisor": 1,
                        "offset": 0
                    }
                    , "bad_address": {
                        "oid": ".1.3.6.1.2.1.4.5.0",
                        "algorithm": "incremental",
                        "multiplier": 1,
                        "divisor": 1,
                        "offset": 0
                    }
                    , "unknown_protocol": {
                        "oid": ".1.3.6.1.2.1.4.7.0",
                        "algorithm": "incremental",
                        "multiplier": 1,
                        "divisor": 1,
                        "offset": 0
                    }
                }
            }
            , "snmp_switch.output": {
                "title": "Switch Output Packets",
                "units": "packets/s",
                "type": "line",
                "priority": 40,
                "family": "IP",
                "dimensions": {
                    "requests": {
                        "oid": ".1.3.6.1.2.1.4.10.0",
                        "algorithm": "incremental",
                        "multiplier": 1,
                        "divisor": 1,
                        "offset": 0
                    }
                    , "discards": {
                        "oid": ".1.3.6.1.2.1.4.11.0",
                        "algorithm": "incremental",
                        "multiplier": -1,
                        "divisor": 1,
                        "offset": 0
                    }
                    , "no_route": {
                        "oid": ".1.3.6.1.2.1.4.12.0",
                        "algorithm": "incremental",
                        "multiplier": -1,
                        "divisor": 1,
                        "offset": 0
                    }
                }
            }
            , "snmp_switch.bandwidth_port": {
                "title": "Switch Bandwidth for port ",
                "titleoid": ".1.3.6.1.2.1.31.1.1.1.18.",
                "units": "kilobits/s",
                "type": "area",
                "priority": 100,
                "family": "ports",
                "multiply_range": [ 1, 24 ],
                "dimensions": {
                    "in": {
                        "oid": ".1.3.6.1.2.1.2.2.1.10.",
                        "algorithm": "incremental",
                        "multiplier": 8,
                        "divisor": 1024,
                        "offset": 0
                    }
                    , "out": {
                        "oid": ".1.3.6.1.2.1.2.2.1.16.",
                        "algorithm": "incremental",
                        "multiplier": -8,
                        "divisor": 1024,
                        "offset": 0
                    }
                }
            }
        }
    }
    ]
}
```
