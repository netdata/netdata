<!--
title: "Vnstat monitoring with Netdata"
custom_edit_url: https://github.com/netdata/netdata/edit/master/collectors/python.d.plugin/vnstat/README.md
sidebar_label: "Vnstat"
-->

# Vnstat monitoring with Netdata

Queries Vnstat database for network usage statistics

## Requirements

-   `vnstat` program

Depending on the configuration, it can produce the following charts per detected interface:

1.  **Average Data Rate: Current vs. Yesterday**

    -   now
    -   yesterday

2.  **Hourly Data Rate: Average Data Transfer Rate Per Hour**

    -   hour1
    -   hour2
    -   hour3
    -   etc.

2.  **Hourly Transfer: Total Data Transfer Per Hour**

    -   hour1
    -   hour2
    -   hour3
    -   etc.

2.  **Daily Data Rate: Average Data Transfer Rate Per Day**

    -   day1
    -   day2
    -   day3
    -   etc.

2.  **Daily Transfer: Total Data Transfer Per Day**

    -   day1
    -   day2
    -   day3
    -   etc.

2.  **Monthly Data Rate: Average Data Transfer Rate Per Month**

    -   month1
    -   month2
    -   month3
    -   etc.

2.  **Monthly Transfer: Total Data Transfer Per Month**

    -   month1
    -   month2
    -   month3
    -   etc.

2.  **Yearly Data Rate: Average Data Transfer Rate Per Year**

    -   year1
    -   year2
    -   year3
    -   etc.

2.  **Yearly Transfer: Total Data Transfer Per Year**

    -   year1
    -   year2
    -   year3
    -   etc.

2.  **Total Transfer:Total RX vs. Total TX**

    -   rx
    -   tx

## Configuration

Edit the `python.d/vnstat.conf` configuration file using `edit-config` from the your agent's [config
directory](/docs/guides/step-by-step/step-04.md#find-your-netdataconf-file), which is typically at `/etc/netdata`.

```bash
cd /etc/netdata   # Replace this path with your Netdata config directory, if different
sudo ./edit-config python.d/vnstat.conf
```

Here is an example for a local server:

```yaml
# Update frequency, in seconds:
update_every: 60

# Interface(s) to show data for (separated by a single space):
interface: 'all'

# Data representation (1 = total data transfer; 2 = average data rate; 0 = both):
data_representation: 1

# Enable d3pie charts (0 = disable charts; 1 = enable charts):
enable_charts: 0 # Note: Maximum of one interface allowed per job, for charts to be enabled

# Limit the amount of data represented (0 = chart disabled; -1 = all data shown):
hours_limit: 24
days_limit: 30
months_limit: 12
years_limit: 10
```

Here is one more example:
```yaml
WAN:
    # Update frequency, in seconds:
    update_every : 60

    # Interface(s) to show data for (separated by a single space):
    interface: 'eth0 ens3'

    # Data representation (1 = total data transfer; 2 = average data rate; 0 = both):
    data_representation: 2

    # Enable d3pie charts (0 = disable charts; 1 = enable charts):
    enable_charts: 0 # Note: Maximum of one interface allowed per job, for charts to be enabled

    # Limit the amount of data represented (0 = chart disabled; -1 = all data shown):
    hours_limit: 6
    days_limit: 7
    months_limit: 6
    years_limit: -1

WireGuard:
    # Update frequency, in seconds:
    update_every : 60

    # Interface(s) to show data for (separated by a single space):
    interface: 'wg0'

    # Data representation (1 = total data transfer; 2 = average data rate; 0 = both):
    data_representation: 0

    # Enable d3pie charts (0 = disable charts; 1 = enable charts):
    enable_charts: 1 # Note: Maximum of one interface allowed per job, for charts to be enabled

    # Limit the amount of data represented (0 = chart disabled; -1 = all data shown):
    hours_limit: 12
    days_limit: 5
    months_limit: -1
    years_limit: 0
```

 *Please Note: It is recommended (but not required) to have the `update_every` and `SaveInterval` values match, with the latter being defined in `/etc/vnstat.conf` on most systems.*

---

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fcollectors%2Fpython.d.plugin%2Fvnstat%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
