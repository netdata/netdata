<!--
title: "Integrations"
sidebar_label: "Integrations"
custom_edit_url: "https://github.com/netdata/netdata/edit/master/docs/category-overview-pages/integrations-overview.md"
description: "Available integrations in Netdata"
learn_status: "Published"
learn_rel_path: "Integrations"
sidebar_position: 60
-->

# Integrations

Netdata's ability to monitor out of the box every potentially useful aspect of a node's operation is unparalleled.
But Netdata also provides out of the box, meaningful charts and alerts for hundreds of applications, with the ability
to be easily extended to monitor anything. See the full list of Netdata's capabilities and how you can extend them in the 
[supported collectors list](https://github.com/netdata/netdata/blob/master/collectors/COLLECTORS.md).

Our out of the box alerts were created by expert professionals and have been validated on the field, countless times.
Use them to trigger [alert notifications](https://github.com/netdata/netdata/blob/master/docs/monitor/enable-notifications.md) 
either centrally, via the 
[Cloud alert notifications](https://github.com/netdata/netdata/blob/master/docs/cloud/alerts-notifications/notifications.md)
, or by configuring individual 
[agent notifications](https://github.com/netdata/netdata/blob/master/health/notifications/README.md).

We designed Netdata with interoperability in mind. The Agent collects thousands of metrics every second, and then what
you do with them is up to you. You can 
[store metrics in the database engine](https://github.com/netdata/netdata/blob/master/database/README.md),
or send them to another time series database for long-term storage or further analysis using
Netdata's [exporting engine](https://github.com/netdata/netdata/edit/master/exporting/README.md).


