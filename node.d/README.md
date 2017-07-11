# Disclaimer

Module configurations are written in JSON and **node.js is required**.

to be edited.

---

The following node.d modules are supported:

# fronius

This module collects metrics from the configured solar power installation from Fronius Symo.
See `netdata/conf.d/node.d/fronius.conf.md` for more details.

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
