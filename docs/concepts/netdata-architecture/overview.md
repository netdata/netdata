<!--
title: "Overview"
sidebar_label: "Overview"
custom_edit_url: "https://github.com/netdata/netdata/blob/master/docs/concepts/netdata-architecture/overview.md"
learn_status: "Published"
learn_topic_type: "Concepts"
learn_rel_path: "Concepts/Netdata architecture"
sidebar_position: "000"
learn_docs_purpose: "Overview page"
-->

Netdata is designed to be both simple to use and flexible for every monitoring, visualization, and troubleshooting use
case:

- **Collect**: Netdata collects all available metrics from your system and applications with 300+ collectors, Kubernetes
  service discovery, and in-depth container monitoring, all while using only 1% CPU and a few MB of RAM. It even
  collects metrics from Windows machines.
- **Visualize**: The dashboard meaningfully presents charts to help you understand the relationships between your
  hardware, operating system, running apps/services, and the rest of your infrastructure. Add nodes to Netdata Cloud for
  a complete view of your infrastructure from a single pane of glass.
- **Monitor**: Netdata's health watchdog uses hundreds of preconfigured alarms to notify you via Slack, email, PagerDuty
  and more when an anomaly strikes. Customize with dynamic thresholds, hysteresis, alarm templates, and role-based
  notifications.
- **Troubleshoot**: 1s granularity helps you detect and analyze anomalies other monitoring platforms might have missed.
  Interactive visualizations reduce your reliance on the console, and historical metrics help you trace issues back to
  their root cause.
- **Store**: Netdata's efficient database engine efficiently stores per-second metrics for days, weeks, or even months.
  Every distributed node stores metrics locally, simplifying deployment, slashing costs, and enriching Netdata's
  interactive dashboards.
- **Export**: Integrate per-second metrics with other time-series databases like Graphite, Prometheus, InfluxDB,
  TimescaleDB, and more with Netdata's interoperable and extensible core.
- **Stream**: Aggregate metrics from any number of distributed nodes in one place for in-depth analysis, including
  ephemeral nodes in a Kubernetes cluster.

To understand more about how and what Netdata can do for your infrastructure, feel free to visit any of the following
topics:

## Learn more

<Grid columns="5">
  <Box
    title="Netdata Architecture">
    <BoxList>
      <BoxListItem to="https://github.com/netdata/netdata/blob/master/docs/concepts/netdata-architecture/distributed-data-architecture.md" title="Distributed data architecture" />
      <BoxListItem to="https://github.com/netdata/netdata/blob/master/docs/concepts/netdata-architecture/high-fidelity-monitoring.md" title="High fidility monitoring" />
      <BoxListItem to="https://github.com/netdata/netdata/blob/master/docs/concepts/netdata-architecture/unlimited-scalability.md" title="Unlimited Scalbility"/>
      <BoxListItem to="https://github.com/netdata/netdata/blob/master/docs/concepts/netdata-architecture/zero-configuration.md" title="Zero configuration"/>
      <BoxListItem to="https://github.com/netdata/netdata/blob/master/docs/concepts/netdata-architecture/high-fidelity-monitoring.md" title="Guided troubleshooting" />
    </BoxList>
  </Box>
</Grid>

