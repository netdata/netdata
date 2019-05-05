# Unlimited metrics

All metrics are important and all metrics should be available when you need them.  

## Why?

Collecting all the metrics breaks the first rule of every monitoring text book: "collect only the metrics you need", "collect only the metrics you understand".

Unfortunately, this does not work! Filtering out most metrics is like reading a book by skipping most of its pages...

For many people, monitoring is about:

- Detecting outages
- Capacity planning

However, **slowdowns are 10 times more common** compared to outages (check slide 14 of [Online Performance is Business Performance ](https://www.slideshare.net/KenGodskind/alertsitetrac) reported by Trac Research/AlertSite). Designing a monitoring system targeting only outages and capacity planning solves just a tiny part of the operational problems we face. Check also [Downtime vs. Slowtime: Which Hurts More?](https://dzone.com/articles/downtime-vs-slowtime-which-hurts-more).

To troubleshoot a slowdown, a lot more metrics are needed. Actually all the metrics are needed, since the real cause of a slowdown is most probably quite complex. If we knew the possible reasons, chances are we would have fixed them before they become a problem.

## What do others do?

Most monitoring solutions, when they are able to detect something, provide just a hint (e.g. "hey, there is a 20% drop in requests per second over the last minute") and they expect us to use the console for determining the root cause.

Of course this introduces a lot more problems: how to troubleshoot a slowdown using the console, if the slowdown lifetime is just a few seconds, randomly spread throughout the day?

You can't! You will spend your entire day on the console, waiting for the problem to happen again while you are logged in. A blame war starts: developers blame the systems, sysadmins blame the hosting provider, someone says it is a DNS problem, another one believes it is network related, etc. We have all experienced this, multiple times...

So, why do monitoring solutions and SaaS providers filter out metrics?

They can't do otherwise!

1. Centralization of metrics depends on metrics filtering, to control monitoring costs. Time-series databases limit the number of metrics collected, because the number of metrics influences their performance significantly. They get congested at scale.
2. It is a lot easier to provide an illusion of monitoring by using a few basic metrics.
3. Troubleshooting slowdowns is the hardest IT problem to solve, so most solutions just avoid it.

## What does netdata do?

Netdata collects, stores and visualizes everything, every single metric exposed by systems and applications.

Due to Netdata's distributed nature, the number of metrics collected does not have any noticeable effect on the performance or the cost of the monitoring infrastructure.

Of course, since netdata is also about [meaningful presentation](meaningful-presentation.md), the number of metrics makes Netdata development slower. We, the Netdata developers, need to have a good understanding of the metrics before adding them into Netdata. We need to organize the metrics, add information related to them, configure alarms for them, so that you, the Netdata users, will have the best out-of-the-box experience and all the information required to kill the console for troubleshooting slowdowns.

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fdocs%2Fwhy-netdata%2Funlimited-metrics&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)]()
