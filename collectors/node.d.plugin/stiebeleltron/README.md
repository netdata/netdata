# stiebel eltron

This module collects metrics from the configured heat pump and hot water installation from Stiebel Eltron ISG web.

**Requirements**
 * Configuration file `stiebeleltron.conf` in the node.d Netdata config dir (default: `/etc/netdata/node.d/stiebeleltron.conf`)
 * Stiebel Eltron ISG web with network access (http), without password login

The charts are configurable, however, the provided default configuration collects the following:

1. **General**
 * Outside temperature in C
 * Condenser temperature in C
 * Heating circuit pressure in bar
 * Flow rate in l/min
 * Output of water and heat pumps in %

2. **Heating**
 * Heat circuit 1 temperature in C (set/actual)
 * Heat circuit 2 temperature in C (set/actual)
 * Flow temperature in C (set/actual)
 * Buffer temperature in C (set/actual)
 * Pre-flow temperature in C

3. **Hot Water**
 * Hot water temperature in C (set/actual)

4. **Room Temperature**
 * Heat circuit 1 room temperature in C (set/actual)
 * Heat circuit 2 room temperature in C (set/actual)

5. **Eletric Reheating**
 * Dual Mode Reheating temperature in C (hot water/heating)

6. **Process Data**
 * Remaining compressor rest time in s

7. **Runtime**
 * Compressor runtime hours (hot water/heating)
 * Reheating runtime hours (reheating 1/reheating 2)

8. **Energy**
 * Compressor today in kWh (hot water/heating)
 * Compressor Total in kWh (hot water/heating)
 
 
### configuration

If no configuration is given, the module will be disabled. Each `update_every` is optional, the default is `10`.

---

[Stiebel Eltron Heat pump system with ISG](https://www.stiebel-eltron.com/en/home/products-solutions/renewables/controller_energymanagement/internet_servicegateway/isg_web.html)

Original author: BrainDoctor (github)

The module supports any metrics that are parseable with RegEx. There is no API that gives direct access to the values (AFAIK), so the "workaround" is to parse the HTML output of the ISG.

### Testing
This plugin has been tested within the following environment:
  * ISG version: 8.5.6
  * MFG version: 12
  * Controller version: 9
  * July (summer time, not much activity)
  * Interface language: English
  * login- and password-less ISG web access (without HTTPS it's useless anyway)
  * Heatpump model: WPL 25 I-2
  * Hot water boiler model: 820 WT 1

So, if the language is set to english, copy the following configuration into `/etc/netdata/node.d/stiebeleltron.conf` and change the `url`s.

In my case, the ISG is relatively slow with responding (at least 1s, but also up to 4s). Collecting metrics every 10s is more than enough for me.

### How to update the config

* The dimensions support variable digits, the default is `1`. Most of the values printed by ISG are using 1 digit, some use 2.
* The dimensions also support the `multiplier` and `divisor` attributes, however the divisor gets overridden by `digits`, if specified. Default is `1`.
* The test string for the regex is always the whole HTML output from the url. For each parameter you need to have a regular expression that extracts the value from the HTML source in the first capture group.
  Recommended: [regexr.com](https://regexr.com/) for testing and matching, [freeformatter.com](https://www.freeformatter.com/json-escape.html) for escaping the newly created regex for the JSON config.

The charts are being generated using the configuration below. So if your installation is in another language or has other metrics, just adapt the structure or regexes.
### Configuration template
```json
{
    "enable_autodetect": false,
    "update_every": 10,
    "pages": [
        {
            "name": "System",
            "id": "system",
            "url": "http://machine.ip.or.dns/?s=1,0",
            "update_every": 10,
            "categories": [ 
		{
                    "id": "eletricreheating",
                    "name": "electric reheating",
                    "charts": [
                        {
                            "title": "Dual Mode Reheating Temperature",
                            "id": "reheatingtemp",
                            "unit": "Celsius",
                            "type": "line",
                            "prio": 1,
                            "dimensions": [
                                {
                                    "name": "Heating",
                                    "id": "dualmodeheatingtemp",
                                    "regex": "DUAL MODE TEMP HEATING<\\\/td>\\s*<td.*>(-?[0-9]+,[0-9]+).*<\\\/td>"
                                },
                                {
                                    "name": "Hot Water",
                                    "id" : "dualmodehotwatertemp",
                                    "regex": "DUAL MODE TEMP DHW<\\\/td>\\s*<td.*>(-?[0-9]+,[0-9]+).*<\\\/td>"
                                }
                            ]
                        }
                    ]
                },
                {
                    "id": "roomtemp",
                    "name": "room temperature",
                    "charts": [
                        {
                            "title": "Heat Circuit 1",
                            "id": "hc1",
                            "unit": "Celsius",
                            "type": "line",
                            "prio": 1,
                            "dimensions": [
                                {
                                    "name": "Actual",
                                    "id": "actual",
                                    "regex": "<tr class=\"even\">\\s*<td.*>ACTUAL TEMPERATURE HC 1<\\\/td>\\s*<td.*>(-?[0-9]+,[0-9]+).*<\\\/td>\\s*<\\\/tr>"
                                },
                                {
                                    "name": "Set",
                                    "id" : "set",
                                    "regex": "<tr class=\"odd\">\\s*<td.*>SET TEMPERATURE HC 1<\\\/td>\\s*<td.*>(-?[0-9]+,[0-9]+).*<\\\/td>\\s*<\\\/tr>"
                                }
                            ]
                        },
                        {
                            "title": "Heat Circuit 2",
                            "id": "hc2",
                            "unit": "Celsius",
                            "type": "line",
                            "prio": 2,
                            "dimensions": [
                                {
                                    "name": "Actual",
                                    "id": "actual",
                                    "regex": "<tr class=\"even\">\\s*<td.*>ACTUAL TEMPERATURE HC 2<\\\/td>\\s*<td.*>(-?[0-9]+,[0-9]+).*<\\\/td>\\s*<\\\/tr>"
                                },
                                {
                                    "name": "Set",
                                    "id" : "set",
                                    "regex": "<tr class=\"odd\">\\s*<td.*>SET TEMPERATURE HC 2<\\\/td>\\s*<td.*>(-?[0-9]+,[0-9]+).*<\\\/td>\\s*<\\\/tr>"
                                }
                            ]
                        }
                    ]
                },
		{
                    "id": "heating",
                    "name": "heating",
                    "charts": [
                        {
                            "title": "Heat Circuit 1",
                            "id": "hc1",
                            "unit": "Celsius",
                            "type": "line",
                            "prio": 1,
                            "dimensions": [
                                {
                                    "name": "Actual",
                                    "id": "actual",
                                    "regex": "<tr class=\"odd\">\\s*<td.*>ACTUAL TEMPERATURE HC 1<\\\/td>\\s*<td.*>(-?[0-9]+,[0-9]+).*<\\\/td>\\s*<\\\/tr>"
                                },
                                {
                                    "name": "Set",
                                    "id" : "set",
                                    "regex": "<tr class=\"even\">\\s*<td.*>SET TEMPERATURE HC 1<\\\/td>\\s*<td.*>(-?[0-9]+,[0-9]+).*<\\\/td>\\s*<\\\/tr>"
                                }
                            ]
                        },
                        {
                            "title": "Heat Circuit 2",
                            "id": "hc2",
                            "unit": "Celsius",
                            "type": "line",
                            "prio": 2,
                            "dimensions": [
                                {
                                    "name": "Actual",
                                    "id": "actual",
                                    "regex": "<tr class=\"odd\">\\s*<td.*>ACTUAL TEMPERATURE HC 2<\\\/td>\\s*<td.*>(-?[0-9]+,[0-9]+).*<\\\/td>\\s*<\\\/tr>"
                                },
                                {
                                    "name": "Set",
                                    "id" : "set",
                                    "regex": "<tr class=\"even\">\\s*<td.*>SET TEMPERATURE HC 2<\\\/td>\\s*<td.*>(-?[0-9]+,[0-9]+).*<\\\/td>\\s*<\\\/tr>"
                                }
                            ]
                        },
                        {
                            "title": "Flow Temperature",
                            "id": "flowtemp",
                            "unit": "Celsius",
                            "type": "line",
                            "prio": 3,
                            "dimensions": [
                                {
                                    "name": "Heating",
                                    "id": "heating",
                                    "regex": "ACTUAL FLOW TEMPERATURE WP<\\\/td>\\s*<td.*>(-?[0-9]+,[0-9]+).*<\\\/td>"
                                },
                                {
                                    "name": "Reheating",
                                    "id" : "reheating",
                                    "regex": "ACTUAL FLOW TEMPERATURE NHZ<\\\/td>\\s*<td.*>(-?[0-9]+,[0-9]+).*<\\\/td>"
                                }
                            ]
                        },
                        {
                            "title": "Buffer Temperature",
                            "id": "buffertemp",
                            "unit": "Celsius",
                            "type": "line",
                            "prio": 4,
                            "dimensions": [
                                {
                                    "name": "Actual",
                                    "id": "actual",
                                    "regex": "ACTUAL BUFFER TEMPERATURE<\\\/td>\\s*<td.*>(-?[0-9]+,[0-9]+).*<\\\/td>"
                                },
                                {
                                    "name": "Set",
                                    "id" : "set",
                                    "regex": "SET BUFFER TEMPERATURE<\\\/td>\\s*<td.*>(-?[0-9]+,[0-9]+).*<\\\/td>"
                                }
                            ]
                        },
                        {
                            "title": "Fixed Temperature",
                            "id": "fixedtemp",
                            "unit": "Celsius",
                            "type": "line",
                            "prio": 5,
                            "dimensions": [
                                {
                                    "name": "Set",
                                    "id" : "setfixed",
                                    "regex": "SET FIXED TEMPERATURE<\\\/td>\\s*<td.*>(-?[0-9]+,[0-9]+).*<\\\/td>"
                                }
                            ]
                        },
                        {
                            "title": "Pre-flow Temperature",
                            "id": "preflowtemp",
                            "unit": "Celsius",
                            "type": "line",
                            "prio": 6,
                            "dimensions": [
                                {
                                    "name": "Actual",
                                    "id": "actualreturn",
                                    "regex": "ACTUAL RETURN TEMPERATURE<\\\/td>\\s*<td.*>(-?[0-9]+,[0-9]+).*<\\\/td>"
                                }
                            ]
                        }
                    ]
                },
		{
                    "id": "hotwater",
                    "name": "hot water",
                    "charts": [
			{
                            "title": "Hot Water Temperature",
                            "id": "hotwatertemp",
                            "unit": "Celsius",
                            "type": "line",
                            "prio": 1,
                            "dimensions": [
                                {
                                    "name": "Actual",
                                    "id": "actual",
                                    "regex": "ACTUAL TEMPERATURE<\\\/td>\\s*<td.*>(-?[0-9]+,[0-9]+).*<\\\/td>"
                                },
                                {
                                    "name": "Set",
                                    "id" : "set",
                                    "regex": "SET TEMPERATURE<\\\/td>\\s*<td.*>(-?[0-9]+,[0-9]+).*<\\\/td>"
                                }
                            ]
                        }
                    ]
                },
		{
                    "id": "general",
                    "name": "general",
                    "charts": [
                        {
                            "title": "Outside Temperature",
                            "id": "outside",
                            "unit": "Celsius",
                            "type": "line",
                            "prio": 1,
                            "dimensions": [
                                {
                                    "name": "Outside temperature",
                                    "id": "outsidetemp",
                                    "regex": "OUTSIDE TEMPERATURE<\\\/td>\\s*<td.*>(-?[0-9]+,[0-9]+).*<\\\/td>\\s*<\\\/tr>"
                                }
                            ]
                        },
                        {
                            "title": "Condenser Temperature",
                            "id": "condenser",
                            "unit": "Celsius",
                            "type": "line",
                            "prio": 2,
                            "dimensions": [
                                {
                                    "name": "Condenser",
                                    "id": "condenser",
                                    "regex": "CONDENSER TEMP\\.<\\\/td>\\s*<td.*>(-?[0-9]+,[0-9]+).*<\\\/td>"
                                }
                            ]
                        },
			{
                            "title": "Heating Circuit Pressure",
                            "id": "heatingcircuit",
                            "unit": "bar",							
                            "type": "line",
                            "prio": 3,
                            "dimensions": [
                                {
                                    "name": "Heating Circuit",
                                    "id": "heatingcircuit",
				    "digits": 2,
                                    "regex": "PRESSURE HTG CIRC<\\\/td>\\s*<td.*>(-?[0-9]+,[0-9]*).*<\\\/td>"
                                }
                            ]
                        },
			{
                            "title": "Flow Rate",
                            "id": "flowrate",
                            "unit": "liters/min",
                            "type": "line",
                            "prio": 4,
                            "dimensions": [
                                {
                                    "name": "Flow Rate",
                                    "id": "flowrate",
				    "digits": 2,
                                    "regex": "FLOW RATE<\\\/td>\\s*<td.*>(-?[0-9]+,[0-9]+).*<\\\/td>"
                                }
                            ]
                        },
			{
                            "title": "Output",
                            "id": "output",
                            "unit": "%",
                            "type": "line",
                            "prio": 5,
                            "dimensions": [
                                {
                                    "name": "Heat Pump",
                                    "id": "outputheatpump",
                                    "regex": "OUTPUT HP<\\\/td>\\s*<td.*>(-?[0-9]+,?[0-9]*).*<\\\/td>"
                                },
				{
                                    "name": "Water Pump",
                                    "id": "intpumprate",
                                    "regex": "INT PUMP RATE<\\\/td>\\s*<td.*>(-?[0-9]+,?[0-9]*).*<\\\/td>"
                                }
                            ]
                        }
                    ]
                }
            ]
        },
	{
            "name": "Heat Pump",
            "id": "heatpump",
            "url": "http://machine.ip.or.dns/?s=1,1",
            "update_every": 10,
            "categories": [
		{
                    "id": "runtime",
                    "name": "runtime",
                    "charts": [
                        {
                            "title": "Compressor",
                            "id": "compressor",
                            "unit": "h",
                            "type": "line",
                            "prio": 1,
                            "dimensions": [
                                {
                                    "name": "Heating",
                                    "id": "heating",
                                    "regex": "RNT COMP 1 HEA<\\\/td>\\s*<td.*>(-?[0-9]+,?[0-9]*)"
                                },
                                {
                                    "name": "Hot Water",
                                    "id" : "hotwater",
                                    "regex": "RNT COMP 1 DHW<\\\/td>\\s*<td.*>(-?[0-9]+,?[0-9]*)"
                                }
                            ]
                        },
			{
                            "title": "Reheating",
                            "id": "reheating",
                            "unit": "h",
                            "type": "line",
                            "prio": 2,
                            "dimensions": [
                                {
                                    "name": "Reheating 1",
                                    "id": "rh1",
                                    "regex": "BH 1<\\\/td>\\s*<td.*>(-?[0-9]+,?[0-9]*)"
                                },
                                {
                                    "name": "Reheating 2",
                                    "id" : "rh2",
                                    "regex": "BH 2<\\\/td>\\s*<td.*>(-?[0-9]+,?[0-9]*)"
                                }
                            ]
                        }
                    ]
                },
		{
                    "id": "processdata",
                    "name": "process data",
                    "charts": [
                        {
                            "title": "Remaining Compressor Rest Time",
                            "id": "remaincomp",
                            "unit": "s",
                            "type": "line",
                            "prio": 1,
                            "dimensions": [
                                {
                                    "name": "Timer",
                                    "id": "timer",
                                    "regex": "COMP DLAY CNTR<\\\/td>\\s*<td.*>(-?[0-9]+,?[0-9]*)"
                                }
                            ]
                        }
                    ]
                },
		{
                    "id": "energy",
                    "name": "energy",
                    "charts": [
                        {
                            "title": "Compressor Today",
                            "id": "compressorday",
                            "unit": "kWh",
                            "type": "line",
                            "prio": 1,
                            "dimensions": [
                                {
                                    "name": "Heating",
                                    "id": "heating",
				    "digits": 3,
                                    "regex": "COMPRESSOR HEATING DAY<\\\/td>\\s*<td.*>(-?[0-9]+,?[0-9]*)"
                                },
				{
                                    "name": "Hot Water",
                                    "id": "hotwater",
				    "digits": 3,
                                    "regex": "COMPRESSOR DHW DAY<\\\/td>\\s*<td.*>(-?[0-9]+,?[0-9]*)"
                                }
                            ]
                        },
			{
                            "title": "Compressor Total",
                            "id": "compressortotal",
                            "unit": "MWh",
                            "type": "line",
                            "prio": 2,
                            "dimensions": [
                                {
                                    "name": "Heating",
                                    "id": "heating",
				    "digits": 3,
                                    "regex": "COMPRESSOR HEATING TOTAL<\\\/td>\\s*<td.*>(-?[0-9]+,?[0-9]*)"
                                },
				{
                                    "name": "Hot Water",
                                    "id": "hotwater",
				    "digits": 3,
                                    "regex": "COMPRESSOR DHW TOTAL<\\\/td>\\s*<td.*>(-?[0-9]+,?[0-9]*)"
                                }
                            ]
                        }
                    ]
                }
            ]
        }
    ]
}
```

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fcollectors%2Fnode.d.plugin%2Fstiebeleltron%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)]()
