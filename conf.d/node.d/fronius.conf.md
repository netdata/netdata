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
            "name": "Solar",
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
