# Linux power supply

> THIS MODULE IS OBSOLETE.
> USE THE [PROC PLUGIN](../../proc.plugin) - IT IS MORE EFFICIENT

---

This module monitors variosu metrics reported by power supply drivers
on Linux.  This allows tracking and alerting on things like remaining
battery capacity.

Depending on the uderlying driver, it may provide the following charts
and metrics:

1. Capacity: The power supply capacity expressed as a percentage.
  * capacity\_now

2. Charge: The charge for the power supply, expressed as microamphours.
  * charge\_full\_design
  * charge\_full
  * charge\_now
  * charge\_empty
  * charge\_empty\_design

3. Energy: The energy for the power supply, expressed as microwatthours.
  * energy\_full\_design
  * energy\_full
  * energy\_now
  * energy\_empty
  * energy\_empty\_design

2. Voltage: The voltage for the power supply, expressed as microvolts.
  * voltage\_max\_design
  * voltage\_max
  * voltage\_now
  * voltage\_min
  * voltage\_min\_design

### configuration

Sample:

```yaml
battery:
  supply: 'BAT0'
  charts: 'capacity charge energy voltage'
```

The `supply` key specifies the name of the power supply device to monitor.
You can use `ls /sys/class/power_supply` to get a list of such devices
on your system.

The `charts` key is a space separated list of which charts to try
to display.  It defaults to trying to display everything.

### notes

* Most drivers provide at least the first chart.  Battery powered ACPI
compliant systems (like most laptops) provide all but the third, but do
not provide all of the metrics for each chart.

* Current, energy, and voltages are reported with a _very_ high precision
by the power\_supply framework.  Usually, this is far higher than the
actual hardware supports reporting, so expect to see changes in these
charts jump instead of scaling smoothly.

* If `max` or `full` attribute is defined by the driver, but not a
corresponding `min or `empty` attribute, then netdata will still provide
the corresponding `min` or `empty`, which will then always read as zero.
This way, alerts which match on these will still work.

---

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fcollectors%2Fpython.d.plugin%2Flinux_power_supply%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)]()
