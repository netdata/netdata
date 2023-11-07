# ipmi_events

## OS: Any

This alert presents the number of events in the IPMI System Event Log (SEL).  
If this alert is received, then the Log contains critical, warning, and informational events.

- This alert is raised in a warning state when the number of events in the IPMI SEL exceed 0
  (in other words, when they exist).

<br>

<details>
<summary>References and Sources</summary>

1. ["ipmitool" manual page](
   https://linux.die.net/man/1/ipmitool)

</details>

### Troubleshooting Section

<details>
<summary>Use "ipmitool"</summary>


> ipmitool is a utility for managing and configuring devices that support
> the Intelligent Platform Management Interface. [Github](https://github.com/ipmitool/ipmitool)

You can view the System Event Log using ipmitool, by running the command:

```
root@netdata~ # ipmitool sel list
```

You can find more info and commands in the [manual page](https://linux.die.net/man/1/ipmitool)
</details>