# Accessing Netdata Dashboards

This section contains documentation on how you can access the Netdata dashboard, which are the same both for the Agent and Cloud.

A user accessing the Netdata dashboard **from the Cloud** will always be presented with the latest Netdata dashboard version.

A user accessing the Netdata dashboard **from the Agent** will, by default, be presented with the latest Netdata dashboard version (the same as Netdata Cloud) except in the following scenarios:
* Agent doesn't have Internet access, and is unable to get the latest Netdata dashboards, as a result it falls back to the Netdata dashboard version that 
was shipped with the agent.
* Users have defined, e.g. through URL bookmark, that they wants to see the previous version of the dashboard (accessible `http://NODE:19999/v1`, replacing `NODE` with the IP address or hostname of your Agent). 

## Main sections

The Netdata dashboard consists of the following main sections:
* [Netdata charts](https://github.com/netdata/netdata/blob/master/docs/cloud/visualize/interact-new-charts.md)
* [Infrastructure Overview](https://github.com/netdata/netdata/blob/master/docs/visualize/overview-infrastructure.md)
* [Nodes view](https://github.com/netdata/netdata/blob/master/docs/cloud/visualize/nodes.md)
* [Custom dashboards](https://learn.netdata.cloud/docs/visualizations/custom-dashboards)
* [Alerts](https://github.com/netdata/netdata/blob/master/docs/monitor/view-active-alarms.md)
* [Anomaly Advisor](https://github.com/netdata/netdata/blob/master/docs/cloud/insights/anomaly-advisor.md)
* [Functions](https://github.com/netdata/netdata/blob/master/docs/cloud/netdata-functions.md)
* [Events feed](https://github.com/netdata/netdata/blob/master/docs/cloud/insights/events-feed.md)

> ⚠️ Some sections of the dashboard, when accessed through the agent,  may require the user to be signed-in to Netdata Cloud or having the Agent claimed to Netdata Cloud for their full functionality. Examples include saving visualization settings on charts or custom dashboards, claiming the node to Netdata Cloud, or executing functions on an Agent.


Documentation for previous Agent dashboard can still be found [here](https://github.com/netdata/netdata/blob/master/web/gui/README.md).