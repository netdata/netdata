<!--
title: "Security"
description: "Netdata aims to respect the security of your systems and the data they create, whether you monitor a single system or an entire infrastructure with Netdata Cloud."
custom_edit_url: https://github.com/netdata/netdata/edit/master/docs/overview/security.md
-->

## Security

Netdata aims to respect the security of your systems and the data they create, whether you monitor a single system or an
entire infrastructure with Netdata Cloud.

## Single-node monitoring

As a standalone monitoring agent, Netdata runs as a normal system user, without special privileges, with read-only
dashboards. Furthermore, Netdata only exposes chart metadata and metric values, not raw data, through unidirectional
data collection processes. Netdata cannot change your services or edit files aside from its own configuration and
database files.

Read more about these conscious [security design decisions](/docs/overview/agent-security.md).

### Anonymous statistics

Starting with v1.30, Netdata collects anonymous usage information by default and sends it to a self-hosted PostHog
instance within the Netdata infrastructure. Read about the information collected, and learn how to-opt, on our
[anonymous statistics](/docs/anonymous-statistics.md) page.

The usage statistics are _vital_ for us, as we use them to discover bugs and prioritize new features. We thank you for
_actively_ contributing to Netdata's future.

### Netdata Registry

The default configuration uses a [public registry](/registry/README.md) hosted at `registry.my-netdata.io`. If you use
that public registry, you submit the following information to a third-party server:

-   The URL where you open the the [dashboard](/docs/dashboard/how-dashboard-works.mdx) in your browser. (via http
    request referrer)
-   The hostname of each node you monitor with Netdata

If sending this information to the central Netdata registry violates your security policies, you can configure Netdata
to [run your own registry](/registry/README.md#run-your-own-registry).

## Infrastructure monitoring with Netdata Cloud

[Data privacy](https://netdata.cloud/data-privacy/) is very important to us. We firmly believe that your data belongs to
you. This is why **we don't store any metric data in Netdata Cloud**.

When you open Netdata Cloud, it queries each node in your infrastructure for relevant data in its _locally-stored
database_ of metrics. These metrics are streamed to Netdata Cloud via the [Agent-Cloud Link](/aclk/README.md). Netdata
Cloud then proxies the data to your browser to create the visualizations. Metrics data pass through our systems, but
they are not stored.

We do however store a limited number of _metadata_ to be able to offer the stunning visualizations and advanced
functionality of Netdata Cloud.

### Metadata

The information we store in Netdata Cloud is the following (using the publicly available demo server `frankfurt.my-netdata.io` as an example):
- The email address you used to sign-up/or sign-in
- For each node claimed to your Spaces in Netdata Cloud:
 - Hostname (as it appears in Netdata Cloud)
 - Information shown in `/api/v1/info`. For example: [https://frankfurt.my-netdata.io/api/v1/info](https://frankfurt.my-netdata.io/api/v1/info).
 - The chart metadata shown in `/api/v1/charts`. For example: [https://frankfurt.my-netdata.io/api/v1/info](https://frankfurt.my-netdata.io/api/v1/info).
 - Alarm configurations shown in `/api/v1/alarms?all`. For example: [https://frankfurt.my-netdata.io/api/v1/alarms?all](https://frankfurt.my-netdata.io/api/v1/alarms?all).
 - Active alarms shown in `/api/v1/alarms`. For example: [https://frankfurt.my-netdata.io/api/v1/alarms](https://frankfurt.my-netdata.io/api/v1/alarms).

How we use metadata:
- The data are stored in our production database on Google Cloud and some of it is also used in BigQuery, our data lake,
  for analytics purposes. These analytics are crucial for our product development process.
- Email is used to identify users in regards to product use and to enrich our tools with product use, such as our CRM.
- This data is only available to Netdata and never to a third party.