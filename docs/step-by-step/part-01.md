---
title: Netdata's building blocks
description: The principles and features that make Netdata a health and performance powerhouse.
---

Netdata is a distributed and real-time _health monitoring and performance troubleshooting toolkit_ for monitoring your
systems and applications.

Because the monitoring agent is small and highly-optimized, you can install it all your physical systems, containers,
IoT devices, and edge devices without disrupting their core function.

By default, and without configuration, Netdata delivers real-time insights into everything happening on the system. All
metrics are automatically-updated, providing interactive dashboards that allow you to dive in, discover anomalies, and
figure out the root cause analysis of any issue.

Best of all, Netdata is entirely free, open-source software! Solo developers and enterprises with thousands of systems
can both use it free of charge. We're hosted on [GitHub](https://github.com/netdata/netdata).

## What you'll learn in this part

In the first part of the Netdata guide, you'll learn about:

-   [Netdata's core features](#netdatas-core-features)
-   [Why you should use Netdata](#why-you-should-use-netdata)
-   [How Netdata has complementary systems, not competitors](#how-netdata-has-complimentary-systems-not-competitors)

Let's get started!

## Netdata's core features

-   A sophisticated **dashboard**, which we'll cover in [part 2](/tutorials/part-02/) of this tutorial. The real-time, 
    highly-granular dashboard, with hundreds of charts, is your main source of information about the health and 
    performance of your systems and applications. We designed the dashboard with anomaly detection and quick 
    analysis in mind. We'll return to dashboard-related topics in both [part 7](/tutorials/part-07/) and 
    [part 8](/tutorials/part-08/).
-   **Netdata Cloud** is our SaaS toolkit that helps Netdata users monitor the health and performance of entire infrastructures. We'll cover Netdata Cloud in [part 3](/tutorials/part-03/) of this tutorial.
-   **No configuration necessary**. Without any configuration, you'll get thousands of real-time metrics and hundreds of alarms designed by our community of sysadmin experts. But you _can_ configure Netdata in a lot of ways, some of which we'll cover in [part 4](/tutorials/part-04).
-   **Distributed, per-system installation**. Instead of centralizing metrics in one location, you install Netdata on _every_ system, and each system is responsible for its metrics. Having distributed agents reduces costs and allows Netdata to go into otherwise inaccessible places, like IoT and edge devices.
-   **Sophisticated health monitoring** to ensure you always know when an anomaly hits. In [part 5](/tutorials/part-05/), we dive into how you can tune alarms, write your own, and enable two types of [alarm notifications](/docs/health/notifications/).
-   **High-speed, low-resource collectors** that allow you to collect thousands of metrics every second while using only a fraction of your system's CPU resources and a few MiB of RAM. 
-   **Long-term metrics storage**. With our new database engine, you can store days, weeks, or months of per-second historical metrics. Or you can archive metrics to another database, like MongoDB or Prometheus. We'll cover all these options in [part 9](/tutorials/part-09).

## Why you should use Netdata

Because you care about the health and performance of your systems and applications, and all of the awesome features we
just mentioned. And it's free!

All these may be valid reasons, but let's step back and talk about Netdata's _principles_ for health monitoring and
performance troubleshooting. We have a lot of [complementary
systems](#how-netdata-has-complimentary-systems-not-competitors), and we think there's a good reason why Netdata should
always be your first choice when troubleshooting an anomaly.

We built Netdata on four principles.

### Per-second data collection

Our first principle is per-second data collection for all metrics.

That matters because you can't monitor a 2-second service-level agreement (SLA) with 10-second metrics. You can't detect
quick anomalies if your metrics don't show them.

How do we solve this? By decentralizing monitoring. Each node collects metrics, triggers alarms, and builds dashboards
locally. That means you can scale Netdata to any size of infrastructure and keep those per-second metrics.

### Unlimited metrics

We believe all metrics are fundamentally important, and all metrics should be available.

If you don't collect the metrics you need or the metrics to help you understand, it's like saying you've read a book
after skipping all but the last ten pages. You only know the ending, not everything that leads to it.

Most monitoring solutions are meant to give the user a hint about the problem, and then send them off to one of a few
dozen console tools to find the root cause. Netdata prefers to give you every piece of information you need to solve any
performance issue.

### Meaningful presentation

We want every part of Netdata's dashboard not only to look good and update every second, but also provide context as to
what you're looking at and why it matters.

The principle of meaningful presentation is fundamental to our dashboard's user experience (UX). We could have put
charts in a grid or hidden some behind tabs or buttons. We instead chose to stack them vertically, on a single page, so
you can visually see how, for example, a jump in disk usage can also increase system load.

Here's an example of a system that's experiencing a disk stress test:

![Screen Shot 2019-10-23 at 15 38
32](https://user-images.githubusercontent.com/1153921/67439589-7f920700-f5ab-11e9-930d-fb0014900d90.png)

> For the curious, here's the command: `stress-ng --fallocate 4 --fallocate-bytes 4g --timeout 1m --metrics --verify
> --times`!

### Immediate results

Finally, Netdata should be usable from the moment it's installed.

As we've talked about, and as you'll learn in the following nine parts, Netdata comes installed with:

-    Auto-detected metrics
-    Human-readable units
-    Metrics that are structured into charts, families, and contexts
-    Automatically generated dashboards
-    Charts designed for visual anomaly detection
-    Hundreds of pre-configured alarms

By standardizing your monitoring infrastructure, Netdata tries to make at least one part of your administration tasks
easy!

## How Netdata has complementary systems, not competitors

We'll cover this quickly, as you're probably eager to get on with using Netdata itself.

We don't want to lock you in to using Netdata by itself, and forever. By supporting [archiving to
backends](/docs/backends/) like Graphite, Prometheus, OpenTSDB, MongoDB, and others, you can use Netdata _in
conjunction_ with software that might seem like our competitors.

We don't want to "wage war" with another monitoring solution, whether it's commercial, open-source, or anything in
between. We want to help people create more extraordinary infrastructures.

## What's next?

We think it's imperative you understand why we built Netdata the way we did. But now that we have that behind us, let's
get right into that dashboard you've heard so much about.

<Button><Link to="/tutorials/part-02/">Next: Get to know Netdata's dashboard <FaAngleDoubleRight /></Link></Button>
