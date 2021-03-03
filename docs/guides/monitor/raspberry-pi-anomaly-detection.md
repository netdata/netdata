<!--
title: "Unsupervised anomaly detection for Raspberry Pi monitoring"
description: ""
image: /img/seo/guides/monitor/raspberry-pi-anomaly-detection.png
author: "Andy Maguire"
author_title: "Senior Machine Learning Engineer"
author_img: "/img/authors/andy-maguire.jpg"
custom_edit_url: https://github.com/netdata/netdata/edit/master/docs/guides/monitor/raspberry-pi-anomaly-detection.md
-->

# Unsupervised anomaly detection for Raspberry Pi monitoring

We love IoT and Edge at Netdata, we also love machine learning and using technology like this to ease the pain of
monitoring increasingly complex systems. So we were quite excited recently when we began to explore what might be
involved in enabling our Python based Anomalies collector on a Raspberry Pi. To our surprise and delight, it's actually
quite straightforward, read on to learn what’s involved (spoiler - it’s just a couple of extra commands that will make
you feel like a pro).

## Install dependencies

First make sure Netdata is using python 3 when it runs the python collectors. Edit the `[plugin:python.d]` section in
your `netdata.conf` file to pass in the `-ppython3` command option. 

```conf
[plugin:python.d]
        # update every = 1
        command options = -ppython3
```

Next we must install some of the underlying libraries used by the python packages the collector depends upon. 

```bash
# install llvm
sudo apt install llvm-9

# install some libs numpy needs (this step might be skippable)
sudo apt-get install libatlas3-base libgfortran5 libatlas-base-dev
```

Now we are ready to install the Python packages used by the collector itself. In this step we pass in the location to find llvm as an environment variable pip will use.

```bash
# become netdata user
sudo su -s /bin/bash netdata

# install python libs and tell it where to find llvm as you pip3 install what is needed
LLVM_CONFIG=llvm-config-9 pip3 install --user llvmlite numpy==1.20.1 netdata-pandas==0.0.32 numba==0.50.1 scikit-learn==0.23.2 pyod==0.8.3
```

## Enable collector

Now we are ready to just enable the collector and restart netdata as normal.

```bash
# turn on the anomalies collector
cd /etc/netdata/
sudo ./edit-config python.d.conf
# set `anomalies: no` to `anomalies: yes`

# restart netdata
sudo systemctl restart netdata
```

And that should be it - after a minute or two once you refresh your netdata dashboard you should see the default
anomalies charts. 

## Overhead on system

Of course one of the most important considerations when trying to do anomaly detection at the edge itself (as opposed to
in a centralized cloud somewhere) is the impact of the monitoring on the system it is monitoring. 

Here, we were again pleasantly surprised to see that the anomalies collector with default configuration was consuming
just about 6.5% of CPU at each run jumping to between 20-30% for a couple of seconds during the retraining step (which
you can configure to happen only once every few hours if you wish).

In terms of the runtime of the collector it was averaging around 250ms during each prediction step jumping to about 8-10
seconds during a retraining step (which is typically fine as it just means a small gap in the anomaly charts for a few
seconds during training steps).

The last consideration then is the amount of RAM the collector needs to store both the models and some of the data
during training. Here it is, as we would expect, using the typical amount of RAM we see on other systems of about 100MB
(jumping a little to 120MB during training).

## What's next?

So, all in all, with a small little bit of extra set up and a small overhead on the Pi itself, the anomalies collector
looks like a potentially useful addition to enable unsupervised anomaly detection on your Pi. 

Now it’s over to you, give it a go, share your use cases and please let us know of any feedback over on our [community
forum](https://community.netdata.cloud/t/anomalies-collector-feedback-megathread/767).  

### Related reference documentation

- [Netdata Agent · Get Netdata](/docs/get/README.md)

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fdocs%2Fguides%2Fmonitor%2Fraspberry-pi-anomaly-detection&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
