# Unlimited metrics

All metrics are important and all metrics should be available when you need them.  

## why?

Collecting all the metrics breaks the first rule of every monitoring text book: "collect only the metrics you need", "collect only the metrics you understand".

Unfortunately, this does not work! Filtering out most metrics is like reading a book by skipping most of its pages...

For many people, monitoring is about:

- Detecting outages
- Capacity planning

However, today **slowdowns are 10 times more common** compared to outages.
Designing a monitoring system targeting only outages and capacity planning,
solves just a tiny part of the operational problems we face.

To troubleshoot a slowdown, a lot more metrics are needed.
Actually all the metrics are needed, since the real cause of a slowdown is most probably quite complex.
If we knew the possible reasons, chances are we should have fixed them before they becomes a problem...

## what others do?

So, why monitoring solutions and SaaS providers filter out metrics?

Well... they can't do otherwise!

1. Time-series databases limit the number of metrics collected, because the number of metrics influences their performance significantly. They get congested at scale.

3. SaaS providers centralize all the metrics so they depend on metrics filtering to control their costs.

At the end of the day, most monitoring solutions provide just a hint
(e.g. "hey, there is a 20% drop in requests per second over the last minute")
and they expect us to use the console for determining the root cause.

Of course this introduces a lot more problems: how to troubleshoot a slowdown
using the console, if the slowdown lifetime is just a few seconds, randomly spread
throughout the day?

You can't! You will spend your entire day on the console, waiting for the problem
to happen again while you are logged in. A blame war starts: developers blame
the systems, sysadmins blame the hosting provider, someone says it is a DNS problem,
another one believes it is network related, etc.
We have all experienced this, multiple times...

So, the monitoring industry limits the number of metrics for 3 reasons:

1. Centralization of metrics depends on metrics filtering for controlling monitoring costs.
2. It is a lot easier to provide an illusion of monitoring by using a few basic metrics.
3. Troubleshooting slowdowns is the hardest IT problem to solve, so most solutions just avoid it.

## what netdata does?

Netdata collects, stores and visualizes everything, every single metric exposed
by systems and applications.

Due to Netdata's distributed nature, the number of metrics collected do not
have any noticeable effect on the performance or the cost of the monitoring
infrastructure.

Of course, since netdata is also about meaningful presentation, the number of
metrics make Netdata development slower. We, the Netdata developers, need to
have a good understanding of the metrics before adding them into Netdata. We need
to organize the metrics, add information related to them, configure alarms for them,
so that you, the Netdata users, will have the best out-of-the-box experience and
all the information required to kill the console for troubleshooting slowdowns.

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fdocs%2Fwhy-netdata%2Funlimited-metrics&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)]()
