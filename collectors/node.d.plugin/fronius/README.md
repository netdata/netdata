# fronius

This module collects metrics from the configured solar power installation from Fronius Symo.

**Requirements**
 * Configuration file `fronius.conf` in the node.d netdata config dir (default: `/etc/netdata/node.d/fronius.conf`)
 * Fronius Symo with network access (http)

It produces per server:

1. **Power**
 * Current power input from the grid (positive values), output to the grid (negative values), in W
 * Current power input from the solar panels, in W
 * Current power stored in the accumulator (if present), in W (in theory, untested)

2. **Consumption**
 * Local consumption in W

3. **Autonomy**
 * Relative autonomy in %. 100 % autonomy means that the solar panels are delivering more power than it is needed by local consumption.
 * Relative self consumption in %. The lower the better

4. **Energy**
 * The energy produced during the current day, in kWh
 * The energy produced during the current year, in kWh

5. **Inverter**
 * The current power output from the connected inverters, in W, one dimension per inverter. At least one is always present.
 
 
### configuration

Sample:

```json
{
    "enable_autodetect": false,
    "update_every": 5,
    "servers": [
        {
            "name": "Symo",
            "hostname": "symo.ip.or.dns",
            "update_every": 5,
            "api_path": "/solar_api/v1/GetPowerFlowRealtimeData.fcgi"
        }
    ]
}
```

If no configuration is given, the module will be disabled. Each `update_every` is optional, the default is `5`.

---

[Fronius Symo 8.2](https://www.fronius.com/en/photovoltaics/products/all-products/inverters/fronius-symo/fronius-symo-8-2-3-m)

The plugin has been tested with a single inverter, namely Fronius Symo 8.2-3-M:

- Datalogger version: 240.162630
- Software version: 3.7.4-6
- Hardware version: 2.4D

Other products and versions may work, but without any guarantees.

Example netdata configuration for node.d/fronius.conf. Copy this section to fronius.conf and change name/ip.
The module supports any number of servers. Sometimes there is a lag when collecting every 3 seconds, so 5 should be okay too. You can modify this per server.
```json
{
    "enable_autodetect": false,
    "update_every": 5,
    "servers": [
        {
            "name": "solar",
            "hostname": "symo.ip.or.dns",
            "update_every": 5,
            "api_path": "/solar_api/v1/GetPowerFlowRealtimeData.fcgi"
        }
    ]
}
```

The output of /solar_api/v1/GetPowerFlowRealtimeData.fcgi looks like this:
```json
{
	"Head" : {
		"RequestArguments" : {},
		"Status" : {
			"Code" : 0,
			"Reason" : "",
			"UserMessage" : ""
		},
		"Timestamp" : "2017-07-05T12:35:12+02:00"
	},
	"Body" : {
		"Data" : {
			"Site" : {
				"Mode" : "meter",
				"P_Grid" : -6834.549847,
				"P_Load" : -1271.450153,
				"P_Akku" : null,
				"P_PV" : 8106,
				"rel_SelfConsumption" : 15.685297,
				"rel_Autonomy" : 100,
				"E_Day" : 35020,
				"E_Year" : 5826076,
				"E_Total" : 14788870,
				"Meter_Location" : "grid"
			},
			"Inverters" : {
				"1" : {
					"DT" : 123,
					"P" : 8106,
					"E_Day" : 35020,
					"E_Year" : 5826076,
					"E_Total" : 14788870
				}
			}
		}
	}
}
```

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fcollectors%2Fnode.d.plugin%2Ffronius%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)]()
