<!--
title: "LAMP stack monitoring (Linux, Apache, MySQL, PHP) with Netdata"
description: "TK"
image: /img/seo/guides/monitor/lamp-stack.png
author: "Joel Hans"
author_title: "Editorial Director, Technical & Educational Resources"
author_img: "/img/authors/joel-hans.jpg"
custom_edit_url: https://github.com/netdata/netdata/edit/master/docs/guides/monitor/lamp-stack.md
-->

# LAMP stack monitoring (Linux, Apache, MySQL, PHP) with Netdata

The LAMP stack is the "hello world" for deploying dynamic web applications. It's performant, flexible, and reliable,
which means a developer or sysadmin won't go far in their career without interacting with the stack and its services.

_LAMP_ is an acronym of the core services that make up the web application: **L**inux, **A**pache, **M**ySQL, and
**P**HP. Linux is the operating system that runs everything, Apache is a web server that responds to HTTP requests,
MySQL is the database that returns information, and PHP is the scripting language used to make the application dynamic.
LAMP stacks are the foundation for tons of end-user applications, with [Wordpress](https://wordpress.org/) being the
most popular.

## Challenge

You've already deployed a LAMP stack, either in testing or production, and want to montitor the performance and
availability of every service to ensure the best possible experience for your end users. Depending on your monitoring
experience, you may not even know what metrics you're looking for, much less how to build a dashboard using query
language. You just need a robust monitoring experience that has the metrics you need without a ton required setup.

## Solution

In this tutorial, you'll set up robust LAMP stack monitoring with Netdata in just a few minutes. When you're done,
you'll have one dashboard to monitor every part of your web application, including each essential LAMP stack service.
This dashboard updates every second with new metrics, and pairs those metrics up with preconfigured alarms to keep you
informed of any errors or odd behavior.

## What you need to get started

To follow this tutorial, you need:

- A physical or virtual Linux system, which we'll call a _node_.
- A functional LAMP stack. There's plenty of tutorials for installing a LAMP stack, like [this
  one](https://www.digitalocean.com/community/tutorials/how-to-install-linux-apache-mysql-php-lamp-stack-ubuntu-18-04)
  from Digital Ocean.

## Install the Netdata Agent

If you don't have the free, open-source [Netdata Agent](/docs/get/README.md) installed on your node yet, get started
with a [single kickstart command](/packaging/installer/methods/kickstart.md):

```bash
bash <(curl -Ss https://my-netdata.io/kickstart.sh)
```

Open your favorite browser and navigate to `http://localhost:19999` or `http://NODE:19999`, replacing `NODE` with the
hostname or IP address of your system, to open the local Agent dashboard.

## Enable Apache monitoring

### Apache web logs

## Enable MySQL monitoring

## Enable PHP monitoring

## View LAMP stack metrics

If the Netdata Agent isn't already open in your browser, open a new tab and navigate to `http://localhost:19999` or
`http://NODE:19999`, replacing `NODE` with the hostname or IP address of your system.

## Get alarms for LAMP stack errors

## What's next?

### Related reference documentation

- 

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fpath%2Fto%2Ffile&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)