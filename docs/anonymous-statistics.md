# Anonymous Statistics

From Netdata v1.12 and above, anonymous usage information is collected by default and sent to Google Analytics. 
The statistics calculated from this information will be used for:

1. **Quality assurance**, to help us understand if Netdata behaves as expected and help us identify repeating issues for certain distributions or environment.

2. **Usage statistics**, to help us focus on the parts of Netdata that are used the most, or help us identify the extend our development decisions influence the community.

Information is sent to Netdata via two different channels:
- Google Tag Manager is used when an agent's dashboard is accessed.
- The script `anonymous-statistics.sh` is executed by the Netdata daemon, when Netdata starts, stops cleanly, or fails.

Both methods are controlled via the same [opt-out mechanism](#opt-out)

## Google tag manager

Google tag manager (GTM) is the recommended way of collecting statistics for new implementations using GA. Unlike the older API, the logic of when to send information to GA and what information to send is controlled centrally.

We have configured GTM to trigger the tag only when the variable `anonymous_statistics` is true. The value of this variable is controlled via the [opt-out mechanism](#opt-out).

To ensure anonymity of the stored information, we have configured GTM's GA variable "Fields to set" as follows: 

|Field Name|Value
|---|---
|page|netdata-dashboard
|hostname|dashboard.my-netdata.io
|anonymizeIp|true
|title|netdata dashboard
|campaignSource|{{machine_guid}}
|campaignMedium|web
|referrer|http://dashboard.my-netdata.io
|Page URL|http://dashboard.my-netdata.io/netdata-dashboard
|Page Hostname|http://dashboard.my-netdata.io
|Page Path|/netdata-dashboard
|location|http://dashboard.my-netdata.io

In addition, the netdata-generated unique machine guid is sent to GA via a custom dimension.
You can verify the effect of these settings by examining the GA `collect` request parameters.

The only thing that's impossible for us to prevent from being **sent** is the URL in the "Referrer" Header of the browser request to GA. However, the settings above ensure that all **stored** URLs and host names are anonymized.

## Anonymous Statistics Script

Every time the daemon is started or stopped and every time a fatal condition is encountered, Netdata uses the anonymous statistics script to collect system information and send it to GA via an http call. The information collected for all events is:
 - Netdata version
 - OS name, version, id, id_like
 - Kernel name, version, architecture
 - Virtualization technology 
 - Containerization technology 

Furthermore, the FATAL event sends the Netdata process & thread name, along with the source code function, source code filename and source code line number of the fatal error.
 
To see exactly what and how is collected, you can review the script template `daemon/anonymous-statistics.sh.in`. The template is converted to a bash script called `anonymous-statistics.sh`, installed under the Netdata `plugins directory`, which is usually `/usr/libexec/netdata/plugins.d`. 

## Opt-Out

To opt-out from sending anonymous statistics, you can create a file called `.opt-out-from-anonymous-statistics` under the user configuration directory (usually `/etc/netdata`). The effect of creating the file is the following:
- The daemon will never execute the anonymous statistics script
- The anonymous statistics script will exit immediately if called via any other way (e.g. shell)
- The Google Tag Manager Javascript snippet will remain in the page, but the linked tag will not be fired. The effect is that no data will ever be sent to GA. 

You can also disable telemetry by passing the option `--disable-telemetry` to any of the installers.