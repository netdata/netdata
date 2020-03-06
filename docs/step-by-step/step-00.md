---
title: The step-by-step Netdata tutorial
description: Learn what Netdata is, what it's capable of, and how it'll help you make faster and more informed decisions about the health and performance of your systems and applications.
---

# The step-by-step Netdata tutorial

Welcome to Netdata! We're glad you're interested in our health monitoring and performance troubleshooting system.

Because Netdata is entirely open-source software, you can use it free of charge, whether you want to monitor one or ten
thousand systems! All our code is hosted on [GitHub](https://github.com/netdata/netdata).

This tutorial is designed to help you understand what Netdata is, what it's capable of, and how it'll help you make
faster and more informed decisions about the health and performance of your systems and applications. If you're
completely new to Netdata, or have never tried health monitoring/performance troubleshooting systems before, this
tutorial is perfect for you.

If you have monitoring experience, or would rather get straight into configuring Netdata to your needs, you can jump
straight into code and configurations with our [getting started guide](../getting-started.md).

> This tutorial contains instructions for Netdata installed on a Linux system. Many of the instructions will work on
> other supported operating systems, like FreeBSD and MacOS, but we can't make any guarantees.

## Where to go if you need help

No matter where you are in this Netdata tutorial, if you need help, head over to our [GitHub
repository](https://github.com/netdata/netdata/). That's where we collect questions from users, help fix their bugs, and
point people toward documentation that explains what they're having trouble with.

Click on the **issues** tab to see all the conversations we're having with Netdata users. Use the search bar to find
previously-written advice for your specific problem, and if you don't see any results, hit the **New issue** button to
send us a question.

Or, if that's too complicated, feel free to send this tutorial's author [an email](mailto:joel@netdata.cloud).

## Before we get started

Let's make sure you have Netdata installed on your system!

> If you already installed Netdata, feel free to skip to [Step 1: Netdata's building blocks](step-01.md).

The easiest way to install Netdata on a Linux system is our `kickstart.sh` one-line installer. Run this on your system
and let it take care of the rest. 

This script will install Netdata from source, keep it up to date with nightly releases, connects to the Netdata
[registry](../../registry/README.md), and sends [_anonymous statistics_](../anonymous-statistics.md) about how you use
Netdata. We use this information to better understand how we can improve the Netdata experience for all our users.

```bash
bash <(curl -Ss https://my-netdata.io/kickstart.sh)
```

Once finished, you'll have Netdata installed, and you'll be set up to get _nightly updates_ to get the latest features,
improvements, and bugfixes.

If this method doesn't work for you, or you want to use a different process, visit our [installation
documentation](../../packaging/installer/README.md) for details.

## Netdata fundamentals

[Step 1. Netdata's building blocks](step-01.md)

In this introductory step, we'll talk about the fundamental ideas, philosophies, and UX decisions behind Netdata.

[Step 2. Get to know Netdata's dashboard](step-02.md)

Visit Netdata's dashboard to explore, manipulate charts, and check out alarms. Get your first taste of visual anomaly
detection.

[Step 3. Monitor more than one system with Netdata](step-03.md)

While the dashboard lets you quickly move from one agent to another, Netdata Cloud is our SaaS solution for monitoring
the health of many systems. We'll cover its features and the benefits of using Netdata Cloud on top of the dashboard.

[Step 4. The basics of configuring Netdata](step-04.md)

While Netdata can monitor thousands of metrics in real-time without any configuration, you may _want_ to tweak some
settings based on your system's resources.

## Intermediate steps

[Step 5. Health monitoring alarms and notifications](step-05.md)

Learn how to tune, silence, and write custom alarms. Then enable notifications so you never miss a change in health
status or performance anomaly.

[Step 6. Collect metrics from more services and apps](step-06.md)

Learn how to enable/disable collection plugins and configure a collection plugin job to add more charts to your Netdata
dashboard and begin monitoring more apps and services, like MySQL, Nginx, MongoDB, and hundreds more.

[Step 7. Netdata's dashboard in depth](step-07.md)

Now that you configured your Netdata monitoring agent to your exact needs, you'll dive back into metrics snapshots,
updates, and the dashboard's settings.

## Advanced steps

[Step 8. Building your first custom dashboard](step-08.md)

Using simple HTML, CSS, and JavaScript, we'll build a custom dashboard that displays essential information in any format
you choose. You can even monitor many systems from a single HTML file.

[Step 9. Long-term metrics storage](step-09.md)

By default, Netdata can store lots of real-time metrics, but you can also tweak our custom database engine to your
heart's content. Want to take your Netdata metrics elsewhere? We're happy to help you archive data to Prometheus,
MongoDB, TimescaleDB, and others.

[Step 10. Set up a proxy](step-10.md)

Run Netdata behind an Nginx proxy to improve performance, and enable TLS/HTTPS for better security.
