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
availability of every service to ensure the best possible experience for your end users. 

Depending on your monitoring experience, you may not even know what metrics you're looking for, much less how to build a
dashboard using query language. You just need a robust monitoring experience that has the metrics you need without a ton
required setup.

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
- Optionally, a [Netdata Cloud](https://app.netdata.cloud/sign-up?cloudRoute=/spaces) account, which you can use to view
  metrics from multiple nodes in one dashboard, and a whole lot more, for free.

## Install the Netdata Agent

If you don't have the free, open-source [Netdata Agent](/docs/get/README.md) installed on your node yet, get started
with a [single kickstart command](/packaging/installer/methods/kickstart.md):

```bash
bash <(curl -Ss https://my-netdata.io/kickstart.sh)
```

The Netdata Agent is now collecting metrics from your node every second. You don't need to jump into the dashboard yet,
but if you're curious, open your favorite browser and navigate to `http://localhost:19999` or `http://NODE:19999`,
replacing `NODE` with the hostname or IP address of your system.

## Enable Apache monitoring

Let's begin by configuring Apache to work with Netdata's [Apache data
collector](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/apache).

Actually, there's nothing for you to do to enable Apache monitoring with Netdata.

These days, Apache comes with `mod_status` enabled by default, and Netdata is smart enough to look for metrics at that
endpoint without you configuring it. Netdata is already collecting [`mod_status`
metrics](https://httpd.apache.org/docs/2.4/mod/mod_status.html) _and_ additional metrics from parsing Apache's
`access.log` file.

## Enable MySQL monitoring

MySQL montioring _does_ take a little more effort, because your database is most likely password-protected. You don't
have to configure Netdata, but rather tell MySQL to allow the `netdata` user to connect without a password. Netdata's
[MySQL data collector](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/mysql) collects metrics
without being able to alter or affect operations in any way.

First, log into the MySQL shell. Then, run the following three commands, one at a time:

```mysql
CREATE USER 'netdata'@'localhost';
GRANT USAGE, REPLICATION CLIENT, PROCESS ON *.* TO 'netdata'@'localhost';
FLUSH PRIVILEGES;
```

Run `sudo systemctl restart netdata`, or the [appropriate alternative for your
system](/docs/configure/start-stop-restart.md), to start collecting dozens of MySQL metrics every second.

## Enable PHP monitoring

Finally, let's set up PHP monitoring. Unlike Apache or MySQL, PHP isn't a service that you can monitor directly, unless
you instrument a PHP-based application with [StatsD](/collectors/statsd.plugin/README.md).

However, if you use [PHP-FPM](https://php-fpm.org/) in your LAMP stack, you can monitor that process with our [PHP-FPM
data collector](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/phpfpm).

Open your PHP-FPM configuration for editing, replacing `7.4` with your version of PHP:

```bash
sudo nano /etc/php/7.4/fpm/pool.d/www.conf
```

> Not sure what version of PHP you're using? Run `php -v`.

Find the line that reads `;pm.status_path = /status` and remove the `;` so it looks like this:

```conf
pm.status_path = /status
```

Next, add a new `/status` endpoint to Apache. Open the Apache configuration file you're using for your LAMP stack.

```bash
sudo nano /etc/apache2/sites-available/your_lamp_stack.conf
```

Add the following to the end of the file, again replacing `7.4` with your version of PHP:

```apache
ProxyPass "/status" "unix:/run/php/php7.4-fpm.sock|fcgi://localhost"
```

Save and close the file. Finally, restart the PHP-FPM, Apache, and Netdata processess.

```bash
sudo systemctl restart php7.4-fpm.service
sudo systemctl restart apache2
sudo systemctl restart netdata
```

As the Netdata Agent starts up again, it automatically connects to the new `127.0.0.1/status` page and starts collecting
per-second PHP-FPM metrics.

## View LAMP stack metrics

If the Netdata Agent isn't already open in your browser, open a new tab and navigate to `http://localhost:19999` or
`http://NODE:19999`, replacing `NODE` with the hostname or IP address of your system.

> If you [signed up](https://app.netdata.cloud/sign-up?cloudRoute=/spaces) for Netdata Cloud earlier, you can also view
> LAMP stack metrics there. Be sure to [claim your node](/docs/get/README.md#claim-your-node-to-netdata-cloud) to start
> streaming metrics to your browser through Netdata Cloud.

Netdata puts all metrics and charts onto a single page, grouped by type in the right-hand menu. You should see four
relevant sections: **Apache local**, **MySQL local**, **PHP-FPM local**, and **web log apache**.

## Get alarms for LAMP stack errors



## What's next?

TK

If you're planning on managing more than one node, or want to take advantage of advanced features, like finding the
source of issues faster with [Metric Correlations](https://learn.netdata.cloud/docs/cloud/insights/metric-correlations),
[sign up](https://app.netdata.cloud/sign-up?cloudRoute=/spaces) for a free Netdata Cloud account.

### Related reference documentation

- [Netdata Agent · Get Netdata](/docs/get/README.md)
- [Netdata Agent · Apache data collector](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/apache)
- [Netdata Agent · MySQL data collector](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/mysql)
- [Netdata Agent · PHP-FPM data collector](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/phpfpm)

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fpath%2Fto%2Ffile&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)