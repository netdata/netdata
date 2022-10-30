<!--
title: "Unlimited scalability"
sidebar_label: "Unlimited scalability"
custom_edit_url: "https://github.com/netdata/netdata/blob/master/docs/concepts/netdata-architecture/unlimited-scalability.md"
sidebar_position: 400
learn_status: "Published"
learn_topic_type: "Concepts"
learn_rel_path: "netdata-architecture"
learn_docs_purpose: "Explain the simplicity of scaling the Netdata Arch to an infinite number of nodes"
-->

Due to our [Distributed architecture approach](https://github.com/netdata/netdata/blob/master/docs/concepts/netdata-architecture/distributed-data-architecture.md), Netdata can seamlessly observe a couple, hundreds or even thousands of
nodes. There are no actual bottlenecks especially if you retain metrics locally in the Agents. You only have to deploy the
Agent in any system that you want to monitor and claim them into the cloud. Netdata Cloud queries only slices of data
when and if you request them on the spot. 

