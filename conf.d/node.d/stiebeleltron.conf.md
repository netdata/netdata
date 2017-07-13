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
  Recommended: [regexr.com](regexr.com) for testing and matching, [freeformatter.com](https://www.freeformatter.com/json-escape.html) for escaping the newly created regex for the JSON config.

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
