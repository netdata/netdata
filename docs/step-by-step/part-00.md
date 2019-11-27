---
title: The step-by-step Netdata tutorial
description: Ten parts and a whole lot of monitoring know-how to share.
---

Welcome to Netdata! We're glad you're interested in our health monitoring and performance troubleshooting system.

Because Netdata is entirely open-source software, you can use it free of charge, whether you want to monitor one or ten
thousand systems! Our code is hosted on [GitHub](https://github.com/netdata/netdata).

This tutorial is designed to help you understand what Netdata is, what it's capable of, and how it'll help you learn
more about your systems and applications. If you're completely new to Netdata, or have never tried health
monitoring/performance troubleshooting systems before, this tutorial is perfect for you.

> If you have monitoring experience, or would rather get straight into configuring Netdata to your needs, you can
> also jump straight into code and configurations with the [getting started guide](/docs/getting-started/).

This tutorial contains instructions for Netdata installed on a Linux system. Many of the instructions will work on other
supported operating systems, like FreeBSD and MacOS, but it is not guaranteed.

## Before we get started...

Let's make sure you have Netdata installed on your system!

> If you already installed Netdata, feel free to skip to [Part 01: Netdata's building blocks](/tutorials/part-01/).

The easiest way to install Netdata on a Linux system is our `kickstart-static64.sh` one-line installer. Run this on your
system and let it take care of the rest.

```bash
bash <(curl -Ss https://my-netdata.io/kickstart-static64.sh)
```

Once finished, you'll have Netdata installed, and you'll be set up to get _nightly updates_ to get the latest features,
improvements, and bugfixes.

If this method doesn't work for you, or you want to use a different process, visit our [installation
documentation](/docs/packaging/installer/).

## Netdata fundamentals

<Button><Link to="/tutorials/part-01/">Part 01: Netdata's building blocks</Link></Button>

In this introductory part, we're talking about the fundamental ideas, philosophies, and UX decisions behind Netdata.

<Button><Link to="/tutorials/part-02/">Part 02: Get to know Netdata's dashboard</Link></Button>

Visit Netdata's dashboard to explore, manipulate charts, and check out alarms. Get your first taste of visual anomaly
detection.

<Button><Link to="/tutorials/part-03/">Part 03: Monitor more than one system with Netdata</Link></Button>

While the dashboard lets you quickly move from one agent to another, Netdata Cloud is our SaaS solution for monitoring
the health of many systems. We'll cover its features and the benefits of using Netdata Cloud on top of the dashboard.

<Button><Link to="/tutorials/part-04/">Part 04: The basics of configuring Netdata</Link></Button>

While Netdata can monitor thousands of metrics in real-time without any configuration, you may _want_ to tweak some
settings based on your system's resources.

## Intermediate tutorials

<Button><Link to="/tutorials/part-05/">Part 05: Health monitoring alarms and notifications</Link></Button>

Learn how to tune, silence, and write custom alarms. Then, enable notifications so you never miss a health status or
performance anomaly.

<Button><Link to="/tutorials/part-06/">Part 06: Collect metrics from more services and apps</Link></Button>

Learn how to enable/disable collection plugins, configure a collection plugin job, and add more charts to your Netdata
dashboard.

<Button><Link to="/tutorials/part-07/">Part 07: Netdata's dashboard in depth</Link></Button>

Now that you configured your Netdata monitoring agent to your exact needs, you'll dive back into metrics snapshots,
updates, and the dashboard's settings.

## Advanced tutorials

<Button><Link to="/tutorials/part-08/">Part 08: Building your first custom dashboard</Link></Button>

Build a dashboard that shows the metrics that matter to you the most, or allow you to monitor many systems from a single
HTML file.

<Button><Link to="/tutorials/part-09/">Part 09: Long-term metrics storage</Link></Button>

Netdata isn't just about real-time metrics—learn how to use our new database engine to store historical data and archive
metrics to a different database.

<Button><Link to="/tutorials/part-10/">Part 10: Set up a proxy</Link></Button>

Run Netdata behind an Nginx proxy to improve performance and security by enabling TLS/HTTPS.