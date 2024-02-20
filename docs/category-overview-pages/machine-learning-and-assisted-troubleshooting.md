# Machine Learning and Anomaly Detection

Machine learning (ML) is a subfield of Artificial Intelligence (AI) that enables computers to learn and improve from experience without being explicitly programmed.

In observability, machine learning can be used to detect patterns and anomalies in large datasets, enabling users to identify potential issues before they become critical.

Machine Learning for observability is usually misunderstood, and frequently leads to unrealistic expectations. Check for example the [presentation Google gave at SreCON19](https://www.usenix.org/conference/srecon19emea/presentation/underwood) explaining that all ideas that Google SREs and DevOps came up with, about the use of Machine Learning in observability were bad, and as Todd notes they should feel bad about it.

At Netdata we are approaching machine learning in a completely different way. Instead of trying to make machine learning do something it cannot achieve, we tried to understand if and what useful insights it can provide and eventually we turned it to an assistant that can improve troubleshooting, reduce mean time to resolution and in many case prevent issues from escalating.

## Design Principles

The following are the high level design principles of Machine Learning in Netdata:

1. **Unsupervised**

   In other words: whatever machine learning can do, it should do it by itself, without any help or assistance from users.

2. **Real-time**

   We understand that Machine Learning will have some impact on resource utilization, especially in CPU utilization, but it shouldn't prevent Netdata from being real-time and high-fidelity.

3. **Integrated**

   Everything achieved with machine learning should be tightly integrated to the infrastructure exploration and troubleshooting practices we are used to.

4. **Assist, Advice, Consult**

   If we can't be sure that a decision made by Machine Learning is 100% accurate, we should use this to assist and consult users in their journey.

   In other words, we don't want to wake up someone at 3 AM, just because a machine learning model detected something.

## Machine Learning per Time-Series

Given the samples recently collected for a time-series, Machine Learning is used to detect if a sample just collected is an outlier or not. 

Since the query combinations are infinite, Netdata detects anomalies at the time-series level, and then combines the anomaly rates of all time-series involved in each query, to provide the anomaly rate for the query.

When a collected sample is an outlier, we set the Anomaly Bit of the collected sample and we store it together with the sample value in the time-series database.

## Multiple Machine Learning Models per Time-Series to Eliminate Noise

Unsupervised machine learning has some noise, random false positives.

To remove this noise, Netdata trains multiple machine learning models for each time-series, covering more than the last 2 days, in total.

Netdata uses all of the available ML models to detect anomalies. So, all machine learning models of a time-series need to agree that a collected sample is an outlier, for it to be marked as an anomaly.

This process removes 99% of the false positives, offering reliable unsupervised anomaly detection. 

## Node Level Anomaly

When a metric becomes anomalous, in many cases a lot other metrics get anomalous too.

For example, an anomaly on a web server may also introduce unusual network bandwidth, cpu usage, memory consumption, disk I/O, context switches, interrupts, etc. If the web server is serving an API that has an application server and a database server we may see anomalies being propagated to them too.

To represent the spread of an anomaly in a node, Netdata computes a **Node Level Anomaly**. This is the percentage of the metrics of a node being concurrently anomalous, vs the total number of metrics of that node.

## Node Anomaly Events

Netdata produces a "node anomaly event" when a the percentage of concurrently anomalous time-series is high enough and persists over time.

This anomaly event signals that there was sufficient evidence among all the time-series that some strange behavior might have been detected in a more global sense across the node.

## What is the Anomaly Bit?

Each sample collected, carries an Anomaly Bit. This bit (true/false) is set when the collected sample found to be an outlier, based on the machine learning models available for it so far.

This bit is embedded into the custom floating point number the Netdata database uses, so it does not introduce any overheads in memory or disk footprint.

The query engine of Netdata uses this bit to compute anomaly rates while it executes normal time-series queries. This eliminates to need for additional queries for anomaly rates, as all `/api/v2` time-series query include anomaly rate information.

## What is the Anomaly Rate (AR)?

The Anomaly Rate of a query, is a percentage, representing the number of samples in the query found anomalous, vs the total number of samples participating in the query.
