<!--
title: "High fidelity monitoring"
sidebar_label: "High fidelity monitoring"
custom_edit_url: "https://github.com/netdata/netdata/blob/master/docs/concepts/netdata-architecture/high-fidelity-monitoring.md"
sidebar_position: 300
learn_status: "Published"
learn_topic_type: "Concepts"
learn_rel_path: "netdata-architecture"
learn_docs_purpose: "Present what high fidelity monitoring is: (real time, high resolution, unlimited, fixed step metric databases)"
-->

Netdata is not just another monitoring solution, it's _the_ high fidelity monitoring solution.

What high fidelity means to us:

1. Real time metrics, view metrics/changes in seconds since their occur.
2. Highest resolution of metrics, observe changes occur between seconds.
3. Fixed step metric collection; quantify your observation windows
4. Unlimited data, search for patterns in data that you don't even believe they are correlated.

## High fidelity in observable systems

To identify problem in your systems you need to have deep understanding of what is going on. In systems where services,
apps, databases and containers change their status in milliseconds you need from your monitoring solution to provide you
the richest data so that system is observable.

## Case study

Imagine that you have a database that has a 900ms delay in random moments, if this is acceptable for your case,
no problem. But what if it's a real time database for a financial institution? You can imagine right now the problem. We
live in a world that any latency can cascade to multiple services and introduce huge delays so this worth your time to
investigate.

How would you begin your troubleshooting? Is it a bottleneck in disks, caching latency, A garbage collection
process?

You need to inspect:

☑ Inspect all the metrics real time.

☑ With the highest possible resolution

☑ Fixed step between two observations of a metric

☑ See the actual effect in multiple resources

WIth Netdata's tools and resources, each of those steps becomes a whole lot easier, faster, cheaper, and more productive. 

