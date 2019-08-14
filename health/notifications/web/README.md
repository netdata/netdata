# Dashboard

The Netdata dashboard shows HTML notifications, when it is open.

Such web notifications look like this:
![image](https://cloud.githubusercontent.com/assets/2662304/18407279/82bac6a6-7714-11e6-847e-c2e84eeacbfb.png)

## Alarms

The netdata GUI has a link for the alarms raised in the system on top menu, when the user access it, (s)he will have the following options:

- Active : The active alarms of Netdata. This option does not include the alarms that were not raised.
- All : In this option the user will have alarms that are monitored by netdata and did not raise an alarm and we will also have the alarms raised.
- Log : This option shows the last 1000 alarms raised by the host.

### Check alarms

Since the version 1.17 , netdata allows the user to open all the charts to check what happened in the exact moment that the alarm was raised, the user can access this feature either using the link sent by the alarm-notification script or using the option log in the dashboard, for this last case it is only necessary to click in the row tha describes the alarm.

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fhealth%2Fnotifications%2Fweb%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)]()
