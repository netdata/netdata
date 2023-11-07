# ipmi_sensors_states

## OS: Any

This alert presents the number of IPMI sensors in the non-nominal state. \
If this alert is received, then there are IPMI sensors in the warning or critical state.

- This alert is raised in a warning state when the amount of sensors in a warning state are greater
  than 0.
- If there are any sensors in a critical state, then the alert is also raised to critical.

<br>

<details>
<summary>References and Sources</summary>

1. ["ipmi-sensors" manual page](
   https://www.gnu.org/software/freeipmi/manpages/man8/ipmi-sensors.8.html)

</details>

### Troubleshooting Section

<details>
<summary>Use "ipmi-sensors" tools</summary>

ipmi-sensors is a free software used to display sensor information (the package name is "
freeipmi-tools").

Here are some useful commands:

> -v, --verbose Output verbose sensor output.  
> This option will output additional information about sensors such as thresholds, ranges,
> numbers, and event/reading type codes.

> --output-sensor-state Output sensor state in output.  
> This will add an additional output reporting if a sensor is in a NOMINAL, WARNING, or CRITICAL
> state. The sensor state is an interpreted value based on the current sensor event. The sensor
> state interpretations are determined by the configuration file
> /etc/freeipmi//freeipmi_interpret_sensor.conf.

You can see more options in the [manual page](
https://www.gnu.org/software/freeipmi/manpages/man8/ipmi-sensors.8.html).

</details>
