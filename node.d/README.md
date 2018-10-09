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

# stiebel eltron

This module collects metrics from the configured heat pump and hot water installation from Stiebel Eltron ISG web.
See `netdata/conf.d/node.d/stiebeleltron.conf.md` for more details.

**Requirements**
 * Configuration file `stiebeleltron.conf` in the node.d netdata config dir (default: `/etc/netdata/node.d/stiebeleltron.conf`)
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

The default configuration is provided in [netdata/conf.d/node.d/stiebeleltron.conf.md](https://github.com/netdata/netdata/blob/master/conf.d/node.d/stiebeleltron.conf.md). Just change the `update_every` (if necessary) and hostnames. **You may have to adapt the configuration to suit your needs and setup** (which might be different).

If no configuration is given, the module will be disabled. Each `update_every` is optional, the default is `10`.

---
