# PagerDuty

[PagerDuty](https://www.pagerduty.com/company/) is the enterprise incident resolution service that integrates with ITOps and DevOps monitoring stacks to improve operational reliability and agility. From enriching and aggregating events to correlating them into incidents, PagerDuty streamlines the incident management process by reducing alert noise and resolution times.

Here is an example of a PagerDuty dashboard with Netdata notifications:

![PagerDuty dashboard with Netdata notifications](https://cloud.githubusercontent.com/assets/19278582/21233877/b466a08a-c2a5-11e6-8d66-ee6eed43818f.png)

To have Netdata send notifications to PagerDuty, you'll first need to set up a PagerDuty `Generic API` service and install the PagerDuty agent on the host running Netdata.  See the following guide for details:

<https://www.pagerduty.com/docs/guides/agent-install-guide/>

During the setup of the `Generic API` PagerDuty service, you'll obtain a `pagerduty service key`.  Keep this **service key** handy.

Once the PagerDuty agent is installed on your host and can send notifications from your host to your `Generic API` service on PagerDuty, add the **service key** to `DEFAULT_RECIPIENT_PD` in `health_alarm_notify.conf`:

```
#------------------------------------------------------------------------------
# pagerduty.com notification options
#
# pagerduty.com notifications require the pagerduty agent to be installed and 
# a "Generic API" pagerduty service.
# https://www.pagerduty.com/docs/guides/agent-install-guide/

# multiple recipients can be given like this:
#              "<pd_service_key_1> <pd_service_key_2> ..."

# enable/disable sending pagerduty notifications
SEND_PD="YES"

# if a role's recipients are not configured, a notification will be sent to
# the "General API" pagerduty.com service that uses this service key.
# (empty = do not send a notification for unconfigured roles):
DEFAULT_RECIPIENT_PD="<service key>"
```

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fhealth%2Fnotifications%2Fpagerduty%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
