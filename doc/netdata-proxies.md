# Netdata proxies

> this is an incomplete page - please help us complete it.

A proxy is a netdata that is receiving metrics from a netdata, and streams them to another netdata.

netdata proxies may or may not maintain a database for the metrics passing through them. When they maintain a database, they can also run health checks (alarms and notifications) for the remote host that is streaming the metrics.

To configure a proxy, configure it as a receiving and a sending netdata at the same time, using [stream.conf](https://github.com/netdata/netdata/blob/master/conf.d/stream.conf).

The sending side of a netdata proxy, connects and disconnects to the final destination of the metrics, following the same pattern of the receiving side.

For a practical example see [Monitoring ephemeral nodes](https://github.com/netdata/netdata/wiki/Monitoring-ephemeral-nodes).
