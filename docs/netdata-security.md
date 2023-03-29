# Security and privacy design

This document serves as the relevant Annex to the [Terms of Service](http://netdata.cloud/service-terms/) and
the Data Processing Addendum, when applicable. It provides more information regarding Netdata’s technical and organizational security and privacy measures.

We have given special attention to all aspects of Netdata, ensuring that everything throughout its operation is as secure as possible. Netdata has been designed with security in mind.

> When running Netdata in environments requiring Payment Card Industry Data Security Standard (**PCI DSS**), Systems and Organization Controls (**SOC 2**),
or Health Insurance Portability and Accountability Act (**HIPAA**) compliance, please keep in mind that 
**even when you use Netdata Cloud, all collected data is always stored inside your infrastructure**. 

Dashboard data you view and alert notifications do travel 
over Netdata Cloud, as they also travel over third party networks, to reach your web browser or the notification integrations you have configured, 
but Netdata Cloud does not store metric data. It only transforms them as they pass through it, aggregating them from multiple Agents and Parents, 
to appear as one data source on your browser.

## Cloud design

### User identification and authorization

Netdata ensures that only an email address is stored to create an account and use the Service. 
User identification and authorization is done 
either via third parties (Google, GitHub accounts), or short-lived access tokens, sent to the user’s email account. 

### Personal Data stored

Netdata ensures that only an email address is stored to create an account and use the Service. The same email 
address is used for Netdata product and marketing communications (via Hubspot and Sendgrid). 

Email addresses are stored in our production database on AWS and copied to Google BigQuery, our data lake, 
for analytics purposes. These analytics are crucial for our product development process.

If the user accepts the use of analytical cookies, the email address is also stored in the systems we use to track the 
usage of the application (Posthog and Gainsight PX)

The IP address used to access Netdata Cloud is stored in web proxy access logs. If the user accepts the use of analytical 
cookies, the IP is also stored in the systems we use to track the usage of the application (Posthog and Gainsight PX). 

### Infrastructure data stored

The metric data that you see in the web browser when using Netdata Cloud is streamed directly from the Netdata Agent 
to the Netdata Cloud dashboard, via the Agent-Cloud link (see [data transfer](#data-transfer)). The data passes through our systems, but it isn’t stored. 

The metadata we do store for each node connected to your Spaces in Netdata Cloud is:
  - Hostname (as it appears in Netdata Cloud)
  - Information shown in `/api/v1/info`. For example: [https://frankfurt.my-netdata.io/api/v1/info](https://frankfurt.my-netdata.io/api/v1/info).
  - Metric metadata information shown in `/api/v1/contexts`. For example: [https://frankfurt.my-netdata.io/api/v1/contexts](https://frankfurt.my-netdata.io/api/v1/contexts).
  - Alarm configurations shown in `/api/v1/alarms?all`. For example: [https://frankfurt.my-netdata.io/api/v1/alarms?all](https://frankfurt.my-netdata.io/api/v1/alarms?all).
  - Active alarms shown in `/api/v1/alarms`. For example: [https://frankfurt.my-netdata.io/api/v1/alarms](https://frankfurt.my-netdata.io/api/v1/alarms).

The infrastructure data is stored in our production database on AWS and copied to Google BigQuery, our data lake, for
 analytics purposes.

### Data transfer

All infrastructure data visible on Netdata Cloud has to pass through the Agent-Cloud link (ACLK) mechanism, which 
securely connects a Netdata Agent to Netdata Cloud. The Netdata agent initiates and establishes an outgoing secure 
WebSocket (WSS) connection to Netdata Cloud. The ACLK is encrypted, safe, and is only established if you connect your node. 

Data is encrypted when in transit between a user and Netdata Cloud using TLS.

### Data retention

Netdata may maintain backups of Netdata Cloud Customer Content, which would remain in place for approximately thirty 
(30) days following a deletion in Netdata Cloud. 

### Data portability and erasure

Netdata will, as necessary to enable the Customer to meet its obligations under Data Protection Law, provide the Customer 
via the availability of Netdata Cloud with the ability to access, retrieve, correct and delete the Personal Data stored in 
Netdata Cloud. The Customer acknowledges that such ability may from time to time be limited due to temporary service outages 
for maintenance or other updates to Netdata Cloud, or technically not feasible. 

To the extent that the Customer, in its fulfillment of its Data Protection Law obligations, is unable to access, retrieve, 
correct or delete Customer Personal Data in Netdata Cloud due to prolonged unavailability of Netdata Cloud due to an issue 
within Netdata’s control, Netdata will where possible use reasonable efforts to provide, correct or delete such Customer Personal Data.

If a Customer is unable to delete Personal Data via the self-services functionality, then Netdata deletes Personal Data upon 
the Customer’s written request, within the timeframe specified in the DPA and in accordance with applicable data protection law. 

#### Delete all personal data

To remove all personal info we have about you (email and activities) you need to delete your cloud account by logging into https://app.netdata.cloud and accessing your profile, at the bottom left of your screen. 


## Agent design

### Your data is safe with Netdata

Netdata collects raw data from many sources. For each source, Netdata uses a plugin that connects to the source (or reads the 
relative files produced by the source), receives raw data and processes them to calculate the metrics shown on Netdata dashboards.

Even if Netdata plugins connect to your database server, or read your application log file to collect raw data, the product of 
this data collection process is always a number of **chart metadata and metric values** (summarized data for dashboard visualization). 
All Netdata plugins (internal to the Netdata daemon, and external ones written in any computer language), convert raw data collected 
into metrics, and only these metrics are stored in Netdata databases, sent to upstream Netdata servers, or archived to external 
time-series databases.

The **raw data** collected by Netdata does not leave the host when collected. **The only data Netdata exposes are chart metadata and metric values.**

This means that Netdata can safely be used in environments that require the highest level of data isolation (like PCI Level 1).

### Your systems are safe with Netdata

We are very proud that **the Netdata daemon runs as a normal system user, without any special privileges**. This is quite an 
achievement for a monitoring system that collects all kinds of system and application metrics.

There are a few cases, however, that raw source data are only exposed to processes with escalated privileges. To support these 
cases, Netdata attempts to minimize and completely isolate the code that runs with escalated privileges.

So, Netdata **plugins**, even those running with escalated capabilities or privileges, perform a **hard coded data collection job**. 
They do not accept commands from Netdata. The communication is **unidirectional** from the plugin towards the Netdata daemon, except 
for Functions (see below).  The original application data collected by each plugin do not leave the process they are collected, are 
not saved and are not transferred to the Netdata daemon. The communication from the plugins to the Netdata daemon includes only chart 
metadata and processed metric values.

Child nodes use the same protocol when streaming metrics to their parent nodes. The raw data collected by the plugins of
child Netdata servers are **never leaving the host they are collected**. The only data appearing on the wire are chart
metadata and metric values. This communication is also **unidirectional**: child nodes never accept commands from
parent Netdata servers (except for Functions). 

[Functions](https://github.com/netdata/netdata/blob/master/docs/cloud/netdata-functions.md) is currently 
the only feature that routes requests back to origin Netdata Agents via Netdata Parents. The feature allows Netdata Cloud to send 
a request to the Netdata Agent data collection plugin running at the 
edge, to provide additional information, such as the process tree of a server, or the long queries of a DB. 

<!-- You have full control over the available functions. For more information see “Controlling Access to Functions” and “Disabling Functions”. -->

### Netdata is read-only

Netdata **dashboards are read-only**. Dashboard users can view and examine metrics collected by Netdata, but cannot 
instruct Netdata to do something other than present the already collected metrics.

Netdata dashboards do not expose sensitive information. Business data of any kind, the kernel version, O/S version, 
application versions, host IPs, etc. are not stored and are not exposed by Netdata on its dashboards.

### Protect Netdata from the internet

Users are responsible to take all appropriate measures to secure their Netdata agent installations and especially the Netdata web user interface and API against unauthorized access. Netdata comes with a wide range of options to 
[secure your nodes](https://github.com/netdata/netdata/blob/master/docs/category-overview-pages/secure-nodes.md) in 
compliance with your organization's security policy.

### Anonymous statistics

#### Netdata registry

The default configuration uses a public [registry](https://github.com/netdata/netdata/blob/master/registry/README.md) under registry.my-netdata.io. 
If you use that public registry, you submit the following information to a third party server: 
 - The URL of the agent's web user interface (via http request referrer)
 - The hostnames of your Netdata servers

If sending this information to the central Netdata registry violates your security policies, you can configure Netdata to 
[run your own registry](https://github.com/netdata/netdata/blob/master/registry/README.md#run-your-own-registry).

#### Anonymous telemetry events

Starting with v1.30, Netdata collects anonymous usage information by default and sends it to a self hosted PostHog instance within the Netdata infrastructure. Read
about the information collected and learn how to opt-out, on our 
[anonymous telemetry events](https://github.com/netdata/netdata/blob/master/docs/anonymous-statistics.md) page.

### Netdata directories

The agent stores data in 6 different directories on your system. 
<details>
<summary>Expand to see each directory's purpose, ownership and permissions</summary>
| path|owner|permissions|Netdata|comments|
|:---|:----|:----------|:------|:-------|
| `/etc/netdata`|user `root`<br/>group `netdata`|dirs `0755`<br/>files `0640`|reads|**Netdata config files**<br/>may contain sensitive information, so group `netdata` is allowed to read them.|
| `/usr/libexec/netdata`|user `root`<br/>group `root`|executable by anyone<br/>dirs `0755`<br/>files `0644` or `0755`|executes|**Netdata plugins**<br/>permissions depend on the file - not all of them should have the executable flag.<br/>there are a few plugins that run with escalated privileges (Linux capabilities or `setuid`) - these plugins should be executable only by group `netdata`.|
| `/usr/share/netdata`|user `root`<br/>group `netdata`|readable by anyone<br/>dirs `0755`<br/>files `0644`|reads and sends over the network|**Netdata web static files**<br/>these files are sent over the network to anyone that has access to the Netdata web server. Netdata checks the ownership of these files (using settings at the `[web]` section of `netdata.conf`) and refuses to serve them if they are not properly owned. Symbolic links are not supported. Netdata also refuses to serve URLs with `..` in their name.|
| `/var/cache/netdata`|user `netdata`<br/>group `netdata`|dirs `0750`<br/>files `0660`|reads, writes, creates, deletes|**Netdata ephemeral database files**<br/>Netdata stores its ephemeral real-time database here.|
| `/var/lib/netdata`|user `netdata`<br/>group `netdata`|dirs `0750`<br/>files `0660`|reads, writes, creates, deletes|**Netdata permanent database files**<br/>Netdata stores here the registry data, health alarm log db, etc.|
| `/var/log/netdata`|user `netdata`<br/>group `root`|dirs `0755`<br/>files `0644`|writes, creates|**Netdata log files**<br/>all the Netdata applications, logs their errors or other informational messages to files in this directory. These files should be log rotated.|
</details>


## Organization processes

### Employee identification and authorization

Netdata operates technical and organizational measures for employee identification and authentication, such as logs, policies, 
assigning distinct usernames for each employee and utilizing password complexity requirements for access to all platforms. 

The COO or HR are the primary system owners for all platforms and may designate additional system owners, as needed. Additional 
user access is also established on a role basis, requires the system owner’s approval, and is tracked by HR. User access to each 
platform is subject to periodic review and testing. When an employee changes roles, HR updates the employee’s access to all systems. 
Netdata uses on-boarding and off-boarding processes to regulate access by Netdata Personnel. 

Second-layer authentication is employed where available, by way of multi-factor authentication. 

Netdata’s IT control environment is based upon industry-accepted concepts, such as multiple layers of preventive and detective 
controls, working in concert to provide for the overall protection of Netdata’s computing environment and data assets. 

### Systems security

Netdata maintains a risk-based assessment security program. The framework for Netdata’s security program includes administrative, 
organizational, technical, and physical safeguards reasonably designed to protect the services and confidentiality, integrity, 
and availability of user data. The program is intended to be appropriate to the nature of the services and the size and complexity 
of Netdata’s business operations.
