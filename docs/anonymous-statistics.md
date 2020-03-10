<!--
---
title: "Anonymous statistics"
custom_edit_url: https://github.com/netdata/netdata/edit/master/docs/anonymous-statistics.md
---
-->

# Anonymous statistics

Starting with v1.12, Netdata collects anonymous usage information by default and sends it to Google Analytics. We use
the statistics gathered from this information for two purposes:

1.  **Quality assurance**, to help us understand if Netdata behaves as expected, and to help us classify repeated
     issues with certain distributions or environments.

2.  **Usage statistics**, to help us interpret how people use the Netdata agent in real-world environments, and to help
     us identify how our development/design decisions influence the community.

Netdata sends information to Google Analytics via two different channels:

-   Google Tag Manager fires when you access an agent's dashboard.
-   The Netdata daemon executes the [`anonymous-statistics.sh`
    script](https://github.com/netdata/netdata/blob/6469cf92724644f5facf343e4bdd76ac0551a418/daemon/anonymous-statistics.sh.in)
    when Netdata starts, stops cleanly, or fails.

You can opt-out from sending anonymous statistics to Netdata through three different [opt-out mechanisms](#opt-out).

## Google tag manager

Google tag manager (GTM) is the recommended way of collecting statistics for new implementations using GA. Unlike the
older API, the logic of when to send information to GA and what information to send is controlled centrally.

We have configured GTM to trigger the tag only when the variable `anonymous_statistics` is true. The value of this
variable is controlled via the [opt-out mechanism](#opt-out).

To ensure anonymity of the stored information, we have configured GTM's GA variable "Fields to set" as follows: 

| Field name     | Value                                              |
| -------------- | -------------------------------------------------- |
| page           | netdata-dashboard                                  |
| hostname       | dashboard.my-netdata.io                            |
| anonymizeIp    | true                                               |
| title          | Netdata dashboard                                  |
| campaignSource | {{machine_guid}}                                   |
| campaignMedium | web                                                |
| referrer       | <http://dashboard.my-netdata.io>                   |
| Page URL       | <http://dashboard.my-netdata.io/netdata-dashboard> |
| Page Hostname  | <http://dashboard.my-netdata.io>                   |
| Page Path      | /netdata-dashboard                                 |
| location       | <http://dashboard.my-netdata.io>                   |

In addition, the Netdata-generated unique machine guid is sent to GA via a custom dimension.
You can verify the effect of these settings by examining the GA `collect` request parameters.

The only thing that's impossible for us to prevent from being **sent** is the URL in the "Referrer" Header of the
browser request to GA. However, the settings above ensure that all **stored** URLs and host names are anonymized.

## Anonymous Statistics Script

Every time the daemon is started or stopped and every time a fatal condition is encountered, Netdata uses the anonymous
statistics script to collect system information and send it to GA via an http call. The information collected for all
events is:

-   Netdata version
-   OS name, version, id, id_like
-   Kernel name, version, architecture
-   Virtualization technology 
-   Containerization technology 

Furthermore, the FATAL event sends the Netdata process & thread name, along with the source code function, source code
filename and source code line number of the fatal error.

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
docs](../packaging/installer/README.md).** You can append this option during the initial installation or a manual
update. You can also export the environment variable `DO_NOT_TRACK` with a non-zero or non-empty value
(e.g: `export DO_NOT_TRACK=1`).

When using Docker, **set your `DO_NOT_TRACK` environment variable to `1`.** You can set this variable with the following
command: `export DO_NOT_TRACK=1`. When creating a container using Netdata's [Docker
image](../packaging/docker/README.md#run-netdata-with-the-docker-command) for the first time, this variable will disable
the anonymous statistics script inside of the container.

Each of these opt-out processes does the following:

-   Prevents the daemon from executing the anonymous statistics script.
-   Forces the anonymous statistics script to exit immediately.
-   Stops the Google Tag Manager Javascript snippet, which remains on the dashboard, from firing and sending any data to
    Google Analytics.
