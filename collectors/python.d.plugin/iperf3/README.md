<!--
title: "Iperf3"
custom_edit_url: https://github.com/netdata/netdata/edit/master/collectors/python.d.plugin/iperf3/README.md
-->

# Iperf3 data collector

A basic iperf3 collector that parse output of iperf log file with
basic regexp.

We are not using json output from iperf since data are available only
after the test is done.

To get the data incrementally, you may want to disable stdout line
buffering, with stdbuf utility for instance, and redirect the logs to
a file:


```
stdbuf -o0 iperf3 -s | tee /tmp/iperfs.log
```
