<!--
title: "Overview"
sidebar_label: "Overview"
custom_edit_url: "https://github.com/netdata/netdata/blob/master/docs/concepts/netdata-cloud/overview.md"
sidebar_position: "1500"
learn_status: "Unpublished"
learn_topic_type: "Concepts"
learn_rel_path: "Concepts/Netdata cloud"
learn_docs_purpose: "Explain the Netdata cloud, operation, principals, purpose, and how Netdata runs it's SAAS Netdata cloud"
-->

Netdata Cloud is a web application that gives you real-time visibility for your entire infrastructure. With Netdata
Cloud, you can view key metrics, insightful charts, and active alarms from all your nodes in a single web interface.
When an anomaly strikes, seamlessly navigate to any node to troubleshoot and discover the root cause with the familiar
Netdata dashboard.

Netdata Cloud is free! You can add an entire infrastructure of nodes, invite all your colleagues, and visualize any
number of metrics, charts, and alarms entirely for free.

While Netdata Cloud offers a centralized method of monitoring your Agents, your metrics data is not stored or
centralized in any way. Metrics data remains with your nodes and is only streamed to your browser, through Cloud, when
you're viewing the Netdata Cloud interface.

Netdata Cloud works in parallel with the open-source Netdata monitoring agent to help you monitor your entire
infrastructure [for free <RiExternalLinkLine className="inline-block"
/>](https://netdata.cloud/pricing/) in real time and troubleshoot problems that threaten the health of your nodes before
they occur.

Netdata Cloud requires the open-source Netdata monitoring agent, which is the basis for the metrics, visualizations, and
alarms that you'll find in Netdata Cloud. Every time you view a node in Netdata Cloud, its metrics and metadata are
streamed to Netdata Cloud, then proxied to your browser, with an infrastructure that
ensures [data privacy <RiExternalLinkLine className="inline-block" />](https://netdata.cloud/privacy/).

To learn more abou the basics of Netdata Cloud's basic features, feel free to peruse the links below.

<Grid columns="1" className="mb-16">
  <Box 
    to="/docs/cloud/get-started" 
    title="Get started with Netdata Cloud"
    cta="Go"
    image={true}>
    Ready to get real-time visibility into your entire infrastructure? This guide will help you get started on Netdata Cloud, from signing in for a free account to connecting your nodes.
  </Box>
</Grid>

## Learn about Netdata Cloud's basic features

<Grid columns="2">
  <Box
    title="Netdata Cloud Basics">
    <BoxList>
      <BoxListItem to="/docs/cloud/visualize/overview" title="Rooms" />
      <BoxListItem to="/docs/cloud/visualize/nodes" title="Views" />
      <BoxListItem to="/docs/cloud/visualize/kubernetes" title="spaces" />
    </BoxList>
  </Box>
</Grid>

*******************************************************************************
