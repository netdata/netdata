# Anonymous Statistics

From Netdata v1.12 and above, anonymous usage information is collected by default and sent to Google Analytics. 
The statistics calculated from this information will be used for:

1.  **Quality assurance**, to help us understand if Netdata behaves as expected and help us identify repeating issues for certain distributions or environment.

2.  **Usage statistics**, to help us focus on the parts of Netdata that are used the most, or help us identify the extend our development decisions influence the community.

Information is sent to Netdata via two different channels:

-   Google Tag Manager is used when an agent's dashboard is accessed.
-   The script `anonymous-statistics.sh` is executed by the Netdata daemon, when Netdata starts, stops cleanly, or
    fails.

Both methods are controlled via the same [opt-out mechanism](#opt-out).

## Google tag manager

Google tag manager (GTM) is the recommended way of collecting statistics for new implementations using GA. Unlike the older API, the logic of when to send information to GA and what information to send is controlled centrally.

We have configured GTM to trigger the tag only when the variable `anonymous_statistics` is true. The value of this variable is controlled via the [opt-out mechanism](#opt-out).

To ensure anonymity of the stored information, we have configured GTM's GA variable "Fields to set" as follows: 

| Field Name|Value|
|----------|-----|
| page|netdata-dashboard|
| hostname|dashboard.my-netdata.io|
| anonymizeIp|true|
| title|Netdata dashboard|
| campaignSource|{{machine_guid}}|
| campaignMedium|web|
| referrer|<http://dashboard.my-netdata.io>|
| Page URL|<http://dashboard.my-netdata.io/netdata-dashboard>|
| Page Hostname|<http://dashboard.my-netdata.io>|
| Page Path|/netdata-dashboard|
| location|<http://dashboard.my-netdata.io>|

In addition, the Netdata-generated unique machine guid is sent to GA via a custom dimension.
You can verify the effect of these settings by examining the GA `collect` request parameters.

The only thing that's impossible for us to prevent from being **sent** is the URL in the "Referrer" Header of the browser request to GA. However, the settings above ensure that all **stored** URLs and host names are anonymized.

## Anonymous Statistics Script

Every time the daemon is started or stopped and every time a fatal condition is encountered, Netdata uses the anonymous statistics script to collect system information and send it to GA via an http call. The information collected for all events is:

-   Netdata version
-   OS name, version, id, id_like
-   Kernel name, version, architecture
-   Virtualization technology 
-   Containerization technology 

Furthermore, the FATAL event sends the Netdata process & thread name, along with the source code function, source code filename and source code line number of the fatal error.

To see exactly what and how is collected, you can review the script template `daemon/anonymous-statistics.sh.in`. The template is converted to a bash script called `anonymous-statistics.sh`, installed under the Netdata `plugins directory`, which is usually `/usr/libexec/netdata/plugins.d`. 

## Opt-out

There are three ways of opting-out from anonymous statistics:

**Create a file called `.opt-out-from-anonymous-statistics`.** This empty file, stored in your Netdata configuration
directory (usually `etc/netdata`), immediately stops the statistics script from running.

**Pass the option `--disable-telemetry` to any of the installer scripts in the [installation
docs](../packaging/installer/README.md).** You can append this option during initial installation or during a manual
update.

**Set your `DO_NOT_TRACK` environmental variable to `1`.** You can set this variable with the following: `export
DO_NOT_TRACK=1`. Read more on the [project's homepage](https://consoledonottrack.com/). This variable works with both
the installer scripts and Docker-based installations.

Each of these opt-out procesess does the following:

-   Prevents the daemon from executing the anonymous statistics script.
-   Forces the anonymous statistics script to exit immediately.
-   Stops the Google Tag Manager Javascript snippet, which remains on the dashboard, from firing and sending any data to
    Google Analytics.
