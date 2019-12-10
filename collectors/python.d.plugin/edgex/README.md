
# EdgeX 

EdgeX is an Open Source IoT Platform, hosted by the Linux Foundation.



![](https://www.edgexfoundry.org/wp-content/uploads/sites/25/2018/09/EdgeX_PlatformArchitectureDiagram-1024x651.png)

## Requirements

EdgeX foundry should be installed on the system. Please see the [Quickstart Guide](https://docs.edgexfoundry.org/Ch-QuickStart.html).

## Charts

As of version 0.2:

- Readings/second
- Events/second
- Absolute number of Events/Readings
- Number of registered devices
- Memory Metrics for core services (edgex-core-data, edgex-core-metadata, edgex-core-command, edgex-support-logging). These metrics are generated using the Golang [runtime Package](https://golang.org/pkg/runtime/#ReadMemStats).

        - Alloc - currently allocated number of bytes on the heap,
        - Mallocs and Frees - number of allocations, deallocations
        - Live objects (mallocs - frees)

## Configuration

The plugin is pre-configured to look at localhost for the services, as Netdata *should* be installed on the device it monitors.

**Default configuration:** 
```
# host:              localhost
# protocol:          http
# port_data:         48080
# port_metadata:     48081
# port_command:      48082
# port_logging:      48060
# events_per_second: true
# number_of_devices: true
# metrics:           true


# events_per_second: # true or false. Enable (or not) the event_count and reading_count aggregation.
# number_of_devices: # true or false. Enable (or not) the aggregation of the  number of edgex devices.
# metrics: # true or false. Enable (or not) the aggregation of memory related metrics.
```


## Troubleshooting

Ensure that the services are up and running and they listen to the default ports. If you have EdgeX-related problems, please do look into the docs and join the community to interact with EdgeX developers.

**Docs:** [Edgex docs](https://docs.edgexfoundry.org/Ch-QuickStart.html)

**Github**: [EdgeX Github](https://github.com/edgexfoundry)

**EdgeX community:** [Slack Channel](https://slack.edgexfoundry.org/)