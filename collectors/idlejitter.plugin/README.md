<!--
title: "idlejitter.plugin"
custom_edit_url: https://github.com/netdata/netdata/edit/master/collectors/idlejitter.plugin/README.md
-->

# idlejitter.plugin

Idle jitter is a measure of delays in timing for user processes caused by scheduling limitations.

## How Netdata measures idle jitter

A thread is spawned that requests to sleep for 20000 microseconds (20ms).
When the system wakes it up, it measures how many microseconds have passed.
The difference between the requested and the actual duration of the sleep, is the idle jitter.
This is done at most 50 times per second, to ensure we have a good average. 

This number is useful:

- In multimedia-streaming environments such as VoIP gateways, where the CPU jitter can affect the quality of the service.
- On time servers and other systems that require very precise timing, where CPU jitter can actively interfere with timing precision.
- On gaming systems, where CPU jitter can cause frame drops and stuttering.
- In cloud infrastructure that can pause the VM or container for a small duration to perform operations at the host.

## Charts

idlejitter.plugin generates the idlejitter chart which measures CPU idle jitter in milliseconds lost per second. 

## Configuration

This chart is available without any configuration. 


