# Immediate results

Metrics are a lot more than name-value pairs over time. It is just not practical to require from all users to have a deep understanding of all metrics for monitoring their systems and applications.

## Why?

There is a plethora of metrics. And each of them has a context, a meaning, a way to be interpreted.

Traditionally, monitoring solutions instruct engineers to collect only the metrics they understand. This is a good strategy as long as you have the skills, the expertise and the experience to select them.

For most people, this is an impossible task. It is just not practical to assume that any engineer will have a deep understanding of how the kernel works, how the networking stack works, how the system manages its memory, how it schedules processes, how web servers work, how databases work, etc.

The result is that for most of the world, monitoring sucks. It is incomplete, inefficient, and in most of the cases only useful for providing an illusion that the infrastructure is being monitored. It is not! According to the [State of Monitoring 2017](http://start.bigpanda.io/state-of-monitoring-report-2017), only 11% of the companies are satisfied with their existing monitoring infrastructure, and on the average they use 6-7 monitoring tools.

But even if all the metrics are collected, an even bigger challenge is revealed: What to do with them? How to use them?

The existing monitoring solutions, assume the engineers will:
 
- Design dashboards
- Configure alarms
- Use a query language to investigate issues

The only problem is that all these have to be configured metric by metric.

What a waste of time and money. Hundreds of thousands of people doing the same thing over and over again, trying to understand what the metrics are, how to visualize them, how to configure alarms for them and how to query them when issues arise.

The monitoring industry believes there is this "IT Operations Hero", a person combining these abilities:

1. He has a deep understanding of IT architectures and he is a skillful SysAdmin.
2. He is a superb Network Administrator (can even read and understand the Linux kernel networking stack).
3. He is an exceptional database administrator.
4. He is fluent in software engineering, capable of understanding the internal workings of applications.
5. He masters Data Science, statistical algorithms and he is fluent in writing advanced mathematical queries to reveal the meaning of metrics.

Of course this person does not exist! So, the world needs help...

## What do others do?

Most of the existing solutions, focus on providing a platform "for building your monitoring". So, they provide the tools to collect metrics, store them, visualize them, check them and query them.

Open-source solutions rely almost entirely on configuration. So, you have to do this metric-by-metric work yourself. The result will reflect your skills, your experience, your understanding.

Monitoring SaaS providers offer a very basic set of pre-configured metrics, dashboards and alarms, and assume again that you will configure the rest. So, once more, the result will reflect your skills, your experience, your understanding.

## What does netdata do?

1. Metrics are auto-detected, so for 99% of the cases data collection works out of the box.
2. Metrics are converted to human readable units.
3. Metrics are structured, organized in charts, families and applications.
4. Dashboards are automatically generated, so all metrics are available for exploration immediately.
5. Dashboards are optimized for visual anomaly detection.
6. Hundreds of pre-configured alarm templates are automatically attached to collected metrics.

The result is that Netdata can be used immediately after installation. Netdata:

- Helps engineers understand and learn what the metrics are.
- Does not require any configuration. Of course there are thousands of options to tweak, but the defaults are pretty good for most systems.
- Does not introduce any query languages or any other technology to be learned. Of course some familiarity with the tool is required, but nothing too complicated.
- Includes all the community expertise and experience for monitoring systems and applications.

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fdocs%2Fwhy-netdata%2Fimmediate-results&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)]()
