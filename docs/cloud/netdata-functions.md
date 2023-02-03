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

For more details please check out documentation on how we use our internal collector to get this from the first collector that exposes
functions - [plugins.d](https://github.com/netdata/netdata/blob/master/collectors/plugins.d/README.md#function).

#### What functions are currently available?

| Function | Description | plugin - module |
| :-- | :-- | :-- |
| processes | Detailed information on the currently running processes on the node. | [apps.plugin](https://github.com/netdata/netdata/blob/master/collectors/apps.plugin/README.md) |

If you have ideas or requests for other functions:
* open a [Feature request](https://github.com/netdata/netdata-cloud/issues/new?assignees=&labels=feature+request%2Cneeds+triage&template=FEAT_REQUEST.yml&title=%5BFeat%5D%3A+) on Netdata Cloud repo
* engage with our community on the [Netdata Discord server](https://discord.com/invite/mPZ6WZKKG2).
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
through [ACLK](https://github.com/netdata/netdata/blob/master/aclk/README.md) this
concern is addressed.

## Related Topics

### **Related Concepts**

- [ACLK](https://github.com/netdata/netdata/blob/master/aclk/README.md)
- [plugins.d](https://github.com/netdata/netdata/blob/master/collectors/plugins.d/README.md)

### Related Tasks

- [Run-time troubleshooting with Functions](https://github.com/netdata/netdata/blob/master/docs/cloud/runtime-troubleshooting-with-functions.md)
