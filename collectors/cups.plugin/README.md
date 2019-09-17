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
