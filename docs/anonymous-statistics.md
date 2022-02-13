<!--
---
title: "Anonymous statistics"
custom_edit_url: https://github.com/netdata/netdata/edit/master/docs/anonymous-statistics.md
---
-->

# Anonymous statistics

By default, Netdata collects anonymous usage information from the open-source monitoring agent using the open-source
product analytics platform [PostHog](https://github.com/PostHog/posthog). We self-host our PostHog instance, which means
your data is never sent or processed by any third parties outside of the Netdata infrastructure.

We are strongly committed to your [data privacy](https://netdata.cloud/data-privacy/).

We use the statistics gathered from this information for two purposes:

1.  **Quality assurance**, to help us understand if Netdata behaves as expected, and to help us classify repeated
     issues with certain distributions or environments.

2.  **Usage statistics**, to help us interpret how people use the Netdata agent in real-world environments, and to help
     us identify how our development/design decisions influence the community.

Netdata collects usage information via two different channels:

-   **Agent dashboard**: We use the [PostHog JavaScript integration](https://posthog.com/docs/integrations/js-integration) (with sensitive event attributes overwritten to be anonymized) to send product usage events when you access an [Agent's dashboard](/web/gui/README.md).
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
statistics script to collect system information and send it to the Netdata PostHog via an http call. The information collected for all
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
-   Failures to connect to the Cloud in case the [connection process](/claim/README.md) has been completed. This includes error codes
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
docs](/packaging/installer/README.md).** You can append this option during the initial installation or a manual
update. You can also export the environment variable `DISABLE_TELEMETRY` with a non-zero or non-empty value
(e.g: `export DISABLE_TELEMETRY=1`).

When using Docker, **set your `DISABLE_TELEMETRY` environment variable to `1`.** You can set this variable with the following
command: `export DISABLE_TELEMETRY=1`. When creating a container using Netdata's [Docker
image](/packaging/docker/README.md#create-a-new-netdata-agent-container) for the first time, this variable will disable
the anonymous statistics script inside of the container.

Each of these opt-out processes does the following:

-   Prevents the daemon from executing the anonymous statistics script.
-   Forces the anonymous statistics script to exit immediately.
-   Stops the PostHog Javascript snippet, which remains on the dashboard, from firing and sending any data to the Netdata PostHog.

## Migration from Google Analytics and Google Tag Manager.

Prior to v1.29.4 we used Google Analytics to capture this information. This led to discomfort with some of our users in sending any product usage data to a third party like Google. It was also not even that useful in terms of generating the insights we needed to help catch bugs early and find opportunities for product improvement as Google Analytics does not allow its users access to the raw underlying data without paying a significant amount of money which would be infeasible for a project like Netdata.

While we migrate fully away from Google Analytics to PostHog there maybe be a small period of time where we run both in parallel before we remove all Google Analytics related code. This is to ensure we can fully test and validate the Netdata PostHog implementation before fully defaulting to it.

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fdocs%2Fanonymous-statistics&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)]()
