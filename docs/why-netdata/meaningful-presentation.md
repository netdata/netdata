# Meaningful presentation

Metrics are a lot more than name-value pairs over time. It is just not practical to require from all users to have a deep understanding of all metrics for monitoring their systems and applications.

## Why?

There is a plethora of metrics. And each of them has a context, a meaning, a way to be interpreted.

Traditionally, monitoring solutions instruct engineers to collect only the metrics they understand. This is a good strategy as long as you have a clear understanding of what you need and you have the skills, the expertise and the experience to select them.

For most people, this is an impossible task. It is just not practical to assume that any engineer will have a deep understanding of how the kernel works, how the networking stack works, how the system manages its memory, how it schedules processes, how web servers work, how databases work, etc.

The result is that for most of the world, monitoring sucks. It is incomplete, inefficient, and in most of the cases only useful for providing an illusion that the infrastructure is being monitored. It is not! According to the [State of Monitoring 2017](http://start.bigpanda.io/state-of-monitoring-report-2017), only 11% of the companies are satisfied with their existing monitoring infrastructure, and on the average they use 6-7 monitoring tools.

But even if all the metrics are collected, an even bigger challenge is revealed: What to do with them? How to use them?

The existing monitoring solutions, assume the engineers will:

-   Design dashboards
-   Configure alarms
-   Use a query language to investigate issues

However, all these have to be configured metric by metric.

The monitoring industry believes there is this "IT Operations Hero", a person combining these abilities:

1.  Has a deep understanding of IT architectures and is a skillful SysAdmin.
2.  Is a superb Network Administrator (can even read and understand the Linux kernel networking stack).
3.  Is an exceptional database administrator.
4.  Is fluent in software engineering, capable of understanding the internal workings of applications.
5.  Masters Data Science, statistical algorithms and is fluent in writing advanced mathematical queries to reveal the meaning of metrics.

Of course this person does not exist!

## What do others do?

Most solutions are based on a time-series database. A database that tracks name-value pairs, over time.

Data collection blindly collects metrics and stores them into the database, dashboard editors query the database to visualize the metrics. They may also provide a query editor, that users can use to query the database by hand.

Of course, it is just not practical to work that way when the database has 10,000 unique metrics. Most of them will be just noise, not because they are not useful, but because no one understands them!

So, they collect very limited metrics. Basic dashboards can be created with these metrics, but for any issue that needs to be troubleshooted, the monitoring system is just not adequate. It cannot help. So, engineers are using the console to access the rest of the metrics and find the root cause.

## What does Netdata do?

In Netdata, the meaning of metrics is incorporated into the database:

1.  all metrics are converted and stored to human-friendly units. This is a data-collection process, not a visualization process. For example, cpu utilization in Netdata is stored as percentage, not as kernel ticks.

2.  all metrics are organized into human-friendly charts, sharing the same context and units (similar to what other monitoring solutions call `cardinality`). So, when Netdata developer collect metrics, they configure the correlation of the metrics right in data collection, which is stored in the database too.

3.  all charts are then organized in families, and chart families are organized in applications. These structures are responsible for providing the menu at the right side of Netdata dashboards for exploring the whole database.

The result is a system that can be browsed by humans, even if the database has 100,000 unique metrics. It is pretty natural for everyone to browse them, understand their meaning and their scope.

Of course, this process makes data collection significantly more time consuming. Netdata developers need to normalize and correlate and categorize every single metric Netdata collects.

But it simplifies everything else. Data collection, metrics database and visualization are de-coupled, thus the query engine is simpler, and the visualization is straight forward.

Netdata goes a step further, by enriching the dashboard with information that is useful for most people. So, to improve clarity and help users be more effective, Netdata includes right in the dashboard the community knowledge and expertise about the metrics. So, that Netdata users can focus on solving their infrastructure problem, not on the technicalities of data collection and visualization. 

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fdocs%2Fwhy-netdata%2Fmeaningful-presentation&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
