<!--
title: "Netdata Functions"
sidebar_label: "Netdata Functions"
custom_edit_url: "https://github.com/netdata/netdata/blob/master/docs/cloud/netdata-functions.md"
sidebar_position: "2800"
learn_status: "Published"
learn_topic_type: "Concepts"
learn_rel_path: "Concepts"
learn_docs_purpose: "Present the Netdata Functions what these are and why they should be used."
-->

Netdata Agent collectors are able to expose functions that can be executed in run-time and on-demand. These will be
executed on the node - host where the function is made
available.

#### What is a function?

Collectors besides the metric collection, storing, and/or streaming work are capable of executing specific routines on
request. These routines will bring additional information
to help you troubleshoot or even trigger some action to happen on the node itself.

A function is a  `key`  -  `value`  pair. The  `key`  uniquely identifies the function within a node. The  `value`  is a
function (i.e. code) to be run by a data collector when
the function is invoked.

For more details please check out documentation on our first collector that exposes
functions - [plugins.d](/docs/nightly/references/collectors-references/plugins.d/#function)

#### How do functions work with streaming?

Via streaming, the definitions of functions are transmitted to a parent node so it knows all the functions available on
any children connected to it.

If the parent node is the one connected to Netdata Cloud it is capable of triggering the call to the respective children
node to run the function.

#### Why are they available only on Netdata Cloud?

Since these functions are able to execute routines on the node and due the potential use cases that they can cover, our
concern is to ensure no sensitive
information or disruptive actions are exposed through the Agent's API.

With the communication between the Netdata Agent and Netdata Cloud being
through [ACLK](https://github.com/netdata/netdata/blob/master/docs/concepts/netdata-agent/aclk.md#ACLK) this
concern is addressed.

## Related Topics

### **Related Concepts**

- [ACLK](https://github.com/netdata/netdata/blob/master/docs/concepts/netdata-agent/aclk.md)
- [plugins.d](https://github.com/netdata/netdata/tree/master/collectors/plugins.d)

### Related Tasks

- [Run-time troubleshooting with Functions](docs/nightly/tasks/operations/runtime-troubleshootting-with-function)
