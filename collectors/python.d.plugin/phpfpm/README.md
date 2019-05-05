# phpfpm

This module will monitor one or more php-fpm instances depending on configuration.

**Requirements:**
 * php-fpm with enabled `status` page
 * access to `status` page via web server

It produces following charts:

1. **Active Connections**
 * active
 * maxActive
 * idle

2. **Requests** in requests/s
 * requests

3. **Performance**
 * reached
 * slow

### configuration

Needs only `url` to server's `status`. The minimum configuration for a single job with a server in localhost is the following:

```yaml
update_every : 3
priority     : 90100

local:
  url     : 'http://localhost/status'
  Multiple jobs can be defined, one for each URL.
```

The stock configuration file explains all other settings, as well as how to define multiple jobs, one for each end point. 
You will notice in the stock configuration's jobs that three different jobs with the same name, but different URLs are defined for localhost. Only one of them will run, assuming at least one of the endpoints is responsive. 

```yaml
 netdata python.d.plugin configuration for PHP-FPM
#
# This file is in YaML format. Generally the format is:
#
# name: value
#
# There are 2 sections:
#  - global variables
#  - one or more JOBS
#
# JOBS allow you to collect values from multiple sources.
# Each source will have its own set of charts.
#
# JOB parameters have to be indented (using spaces only, example below).

# ----------------------------------------------------------------------
# Global Variables
# These variables set the defaults for all JOBs, however each JOB
# may define its own, overriding the defaults.

# update_every sets the default data collection frequency.
# If unset, the python.d.plugin default is used.
# update_every: 1

# priority controls the order of charts at the netdata dashboard.
# Lower numbers move the charts towards the top of the page.
# If unset, the default for python.d.plugin is used.
# priority: 60000

# penalty indicates whether to apply penalty to update_every in case of failures.
# Penalty will increase every 5 failed updates in a row. Maximum penalty is 10 minutes.
# penalty: yes

# autodetection_retry sets the job re-check interval in seconds.
# The job is not deleted if check fails.
# Attempts to start the job are made once every autodetection_retry.
# This feature is disabled by default.
# autodetection_retry: 0


# ----------------------------------------------------------------------
# JOBS (data collection sources)
#
# The default JOBS share the same *name*. JOBS with the same name
# are mutually exclusive. Only one of them will be allowed running at
# any time. This allows autodetection to try several alternatives and
# pick the one that works.
#
# Any number of jobs is supported.
#
# All python.d.plugin JOBS (for all its modules) support a set of
# predefined parameters. These are:
#
# job_name:
#     name: myname            # the JOB's name as it will appear at the
#                             # dashboard (by default is the job_name)
#                             # JOBs sharing a name are mutually exclusive
#     update_every: 1         # the JOB's data collection frequency
#     priority: 60000         # the JOB's order on the dashboard
#     penalty: yes            # the JOB's penalty
#     autodetection_retry: 0  # the JOB's re-check interval in seconds
#
# Additionally to the above, PHP-FPM also supports the following:
#
#     url: 'URL'       # the URL to fetch the status stats
#                      # Be sure and include ?full&status at the end of the url
#
# if the URL is password protected, the following are supported:
#
#     user: 'username'
#     pass: 'password'
#

# ----------------------------------------------------------------------
# AUTO-DETECTION JOBS
# only one of them will run (they have the same name)

localhost:
  name : 'local'
  url  : "http://localhost/status?full&json"

localipv4:
  name : 'local'
  url  : "http://127.0.0.1/status?full&json"

localipv6:
  name : 'local'
  url  : "http://[::1]/status?full&json"

```



---

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fcollectors%2Fpython.d.plugin%2Fphpfpm%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)]()
