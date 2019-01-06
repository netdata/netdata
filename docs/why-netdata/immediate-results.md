# Immediate results

Most of our infrastructure is based on standardized systems and applications. 
 
It is a tremendous waste of time and effort, in a global scale, to require from all users to configure their infrastructure dashboards and alarms metric by metric.

## Why?

Most of the existing monitoring solutions, focus on providing a platform "for building your monitoring". So, they provide the tools to collect metrics, store them, visualize them, check them and query them. And we are all expected to go through this process.

However, most of our infrastructure is standardized. We run well known Linux distributions, the same kernel, the same database, the same web server, etc.

So, why can't we have a monitoring system that can be installed and instantly provide feature rich dashboards and alarms about everything we use? Is there any reason you would like to monitor your web server differently than me?

What a waste of time and money! Hundreds of thousands of people doing the same thing over and over again, trying to understand what the metrics are, how to visualize them, how to configure alarms for them and how to query them when issues arise.

## What do others do?

Open-source solutions rely almost entirely on configuration. So, you have to go through endless metric-by-metric configuration yourself. The result will reflect your skills, your experience, your understanding.

Monitoring SaaS providers offer a very basic set of pre-configured metrics, dashboards and alarms. They assume that you will configure the rest you may need. So, once more, the result will reflect your skills, your experience, your understanding.

## What does netdata do?

1. Metrics are auto-detected, so for 99% of the cases data collection works out of the box.
2. Metrics are converted to human readable units, right after data collection, before storing them into the database.
3. Metrics are structured, organized in charts, families and applications, so that they can be browsed.
4. Dashboards are automatically generated, so all metrics are available for exploration immediately after installation.
5. Dashboards are not just visualizing metrics; they are a tool, optimized for visual anomaly detection.
6. Hundreds of pre-configured alarm templates are automatically attached to collected metrics.

The result is that Netdata can be used immediately after installation!

Netdata:

- Helps engineers understand and learn what the metrics are.
- Does not require any configuration. Of course there are thousands of options to tweak, but the defaults are pretty good for most systems.
- Does not introduce any query languages or any other technology to be learned. Of course some familiarity with the tool is required, but nothing too complicated.
- Includes all the community expertise and experience for monitoring systems and applications.

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fdocs%2Fwhy-netdata%2Fimmediate-results&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)]()
