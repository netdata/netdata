# PCI DSS, SOC 2 and HIPAA environments

When running Netdata in environments requiring Payment Card Industry Data Security Standard (PCI DSS), Systems and Organization Controls (SOC 2),
or Health Insurance Portability and Accountability Act (HIPAA) compliance, please keep in mind the following:

## All collected data is always stored inside your infrastructure

Even when you use Netdata Cloud, your data is never stored outside your infrastructure. Dashboard data you view and alert notifications do travel 
over Netdata Cloud, as they also travel over third party networks, to reach your web browser or the notification integrations you have configured, 
but Netdata Cloud does not store metric data. It only transforms them as they pass through it, aggregating them from multiple Agents and Parents, 
to appear as one data source on your browser.

For more information see [Metric Retention - Database](https://github.com/netdata/netdata/blob/master/database/README.md) and 
[Data Netdata Cloud Stores and Processes](https://github.com/netdata/netdata/blob/master/docs/cloud/data-privacy.md#data-netdata-cloud-stores-and-processes).

## Netdata Parents can be used as Web Application Firewalls (WAFs) for accessing your monitoring data

The Netdata Agents you install on your production systems do not need direct access to the Internet. Even when you use Netdata Cloud, you can appoint 
one or more Netdata Parents to act as border gateways or application firewalls, isolating your production systems from the rest of the world. Netdata 
Parents receive metric data from Netdata Agents or other Netdata Parents on one side, and serve most queries using their own copy of the data to satisfy 
dashboard requests on the other side.

For more information see [Streaming and replication](https://github.com/netdata/netdata/blob/master/docs/metrics-storage-management/enable-streaming.md).

[Functions](https://github.com/netdata/netdata/blob/master/docs/cloud/netdata-functions.md) is currently the only feature that routes requests back to 
origin Netdata Agents via Netdata Parents. The feature allows Netdata Cloud to send a request to the Netdata Agent data collection plugin running at the 
edge, to provide additional information, such as the process tree of a server, or the long queries of a DB. 

<!-- You have full control over the available functions. For more information see “Controlling Access to Functions” and “Disabling Functions”. -->
