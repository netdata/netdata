<!--
title: "cups.plugin"
custom_edit_url: https://github.com/netdata/netdata/edit/master/collectors/cups.plugin/README.md
-->

# cups.plugin

`cups.plugin` collects Common Unix Printing System (CUPS) metrics.

## Prerequisites

This plugin needs a running local CUPS daemon (`cupsd`). This plugin does not need any configuration. Supports cups since version 1.7.

## Charts

`cups.plugin` provides one common section `destinations` and one section per destination. 

> Destinations in CUPS represent individual printers or classes (collections or pools) of printers (<https://www.cups.org/doc/cupspm.html#working-with-destinations>)

The section `server` provides these charts:

1.  **destinations by state**

    -   idle
    -   printing
    -   stopped

2.  **destinations by options**

    -   total
    -   accepting jobs
    -   shared

3.  **total job number by status**

    -   pending
    -   processing
    -   held

4.  **total job size by status**

    -   pending
    -   processing
    -   held

For each destination the plugin provides these charts:

1.  **job number by status**

    -   pending
    -   held
    -   processing

2.  **job size by status**

    -   pending
    -   held
    -   processing

At the moment only job status pending, processing, and held are reported because we do not have a method to collect stopped, canceled, aborted and completed jobs which scales.

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fcollectors%2cups.plugin%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)]()
