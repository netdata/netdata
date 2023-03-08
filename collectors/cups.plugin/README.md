<!--
title: "Printers (cups.plugin)"
custom_edit_url: "https://github.com/netdata/netdata/edit/master/collectors/cups.plugin/README.md"
sidebar_label: "cups.plugin"
learn_status: "Published"
learn_topic_type: "References"
learn_rel_path: "Integrations/Monitor/Remotes/Devices"
-->

# Printers (cups.plugin)

`cups.plugin` collects Common Unix Printing System (CUPS) metrics.

## Prerequisites

This plugin needs a running local CUPS daemon (`cupsd`). This plugin does not need any configuration. Supports cups since version 1.7.

If you installed Netdata using our native packages, you will have to additionally install `netdata-plugin-cups` to use this plugin for data collection. It is not installed by default due to the large number of dependencies it requires.

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


