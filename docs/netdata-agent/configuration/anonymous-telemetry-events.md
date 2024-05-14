<!--
title: "Anonymous telemetry events"
custom_edit_url: https://github.com/netdata/netdata/edit/master/docs/anonymous-statistics.md
sidebar_label: "Anonymous telemetry events"
learn_status: "Published"
learn_rel_path: "Configuration"
-->

# Anonymous telemetry events

By default, Netdata collects anonymous usage information from the open-source monitoring agent. For agent events like start,stop,crash etc we use our own cloud function in GCP. For frontend telemetry (pageviews etc.) on the agent dashboard itself we use the open-source
product analytics platform [PostHog](https://github.com/PostHog/posthog).

We are strongly committed to your [data privacy](https://netdata.cloud/privacy/).

We use the statistics gathered from this information for two purposes:

1.  **Quality assurance**, to help us understand if Netdata behaves as expected, and to help us classify repeated
     issues with certain distributions or environments.

2.  **Usage statistics**, to help us interpret how people use the Netdata agent in real-world environments, and to help
     us identify how our development/design decisions influence the community.

Netdata collects usage information via two different channels:

-   **Agent dashboard**: We use the [PostHog JavaScript integration](https://posthog.com/docs/integrations/js-integration) (with sensitive event attributes overwritten to be anonymized) to send product usage events when you access an [Agent's dashboard](https://github.com/netdata/netdata/blob/master/docs/dashboards-and-charts/README.md).
-   **Agent backend**: The `netdata` daemon executes the [`anonymous-statistics.sh`](https://github.com/netdata/netdata/blob/6469cf92724644f5facf343e4bdd76ac0551a418/daemon/anonymous-statistics.sh.in) script when Netdata starts, stops cleanly, or fails.

You can opt-out from sending anonymous statistics to Netdata through three different [opt-out mechanisms](#opt-out).

## Agent Dashboard - PostHog JavaScript

When you kick off an Agent dashboard session by visiting `http://NODE:19999`, Netdata initializes a PostHog session and masks various event attributes.

_Note_: You can see the relevant code in the [dashboard repository](https://github.com/netdata/dashboard/blob/master/src/domains/global/sagas.ts#L107) where the `window.posthog.register()` call is made.  

```JavaScript
window.posthog.register({
    distinct_id: machineGuid,
    $ip: "127.0.0.1",
    $current_url: "agent dashboard",
    $pathname: "netdata-dashboard",
    $host: "dashboard.netdata.io",
})
```

In the above snippet a Netdata PostHog session is initialized and the `ip`, `current_url`, `pathname` and `host` attributes are set to constant values for all events that may be sent during the session. This way, information like the IP or hostname of the Agent will not be sent as part of the product usage event data.

We have configured the dashboard to trigger the PostHog JavaScript code only when the variable `anonymous_statistics` is true. The value of this
variable is controlled via the [opt-out mechanism](#opt-out).

## Agent Backend - Anonymous Statistics Script

Every time the daemon is started or stopped and every time a fatal condition is encountered, Netdata uses the anonymous
statistics script to collect system information and send it to the Netdata telemetry cloud function via an http call. The information collected for all
events is:

-   Netdata version
-   OS name, version, id, id_like
-   Kernel name, version, architecture
-   Virtualization technology 
-   Containerization technology 

Furthermore, the FATAL event sends the Netdata process & thread name, along with the source code function, source code
filename and source code line number of the fatal error.

Starting with v1.21, we additionally collect information about:

-   Failures to build the dependencies required to use Cloud features.
-   Unavailability of Cloud features in an agent.
-   Failures to connect to the Cloud in case the [connection process](https://github.com/netdata/netdata/blob/master/src/claim/README.md) has been completed. This includes error codes
    to inform the Netdata team about the reason why the connection failed.

To see exactly what and how is collected, you can review the script template `daemon/anonymous-statistics.sh.in`. The
template is converted to a bash script called `anonymous-statistics.sh`, installed under the Netdata `plugins
directory`, which is usually `/usr/libexec/netdata/plugins.d`. 

## Opt-out

You can opt-out from sending anonymous statistics to Netdata through three different opt-out mechanisms:

**Create a file called `.opt-out-from-anonymous-statistics`.** This empty file, stored in your Netdata configuration
directory (usually `/etc/netdata`), immediately stops the statistics script from running, and works with any type of
installation, including manual, offline, and macOS installations. Create the file by running `touch
.opt-out-from-anonymous-statistics` from your Netdata configuration directory.

**Pass the option `--disable-telemetry` to any of the installer scripts in the [installation
docs](https://github.com/netdata/netdata/blob/master/packaging/installer/README.md).** You can append this option during the initial installation or a manual
update. You can also export the environment variable `DISABLE_TELEMETRY` with a non-zero or non-empty value
(e.g: `export DISABLE_TELEMETRY=1`).

When using Docker, **set your `DISABLE_TELEMETRY` environment variable to `1`.** You can set this variable with the following
command: `export DISABLE_TELEMETRY=1`. When creating a container using Netdata's [Docker
image](https://github.com/netdata/netdata/blob/master/packaging/docker/README.md#create-a-new-netdata-agent-container) for the first time, this variable will disable
the anonymous statistics script inside of the container.

Each of these opt-out processes does the following:

-   Prevents the daemon from executing the anonymous statistics script.
-   Forces the anonymous statistics script to exit immediately.
-   Stops the PostHog JavaScript snippet, which remains on the dashboard, from firing and sending any data to the Netdata PostHog.


