# Exim monitoring with Netdata

Simple module executing `exim -bpc` to grab exim queue.
This command can take a lot of time to finish its execution thus it is not recommended to run it every second.
test

**Requirements**

The module uses `exim` binary which can only be executed as root by default. We need to allow other user that can run `exim` binary. We solving that adding `queue_list_requires_admin` statement in exim configuration and set to `false` because is `true` by default. On many Linux OS default location of `exim` configuration was in `/etc/exim.conf`.

1. Edit config with favor editor and add
`queue_list_requires_admin = false`
2. Restart exim and netdata

*WHM (CPanel) server*

On the WHM server `exim` we can reconfigure over WHM interface with next steps.

1. Login to WHM
2. Navigate to 
Service Configuration --> Exim Configuration Manager --> tab Advanced Editor
3. Scroll down to button `Add additional configuration setting`and click on it.
4. In the new dropdown which will appear above we need to find and choose:
`queue_list_requires_admin` and set to `false` 
5. Scroll to the end and click `Save` button.


It produces only one chart:

1.  **Exim Queue Emails**

    -   emails

Configuration is not needed.

---

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fcollectors%2Fpython.d.plugin%2Fexim%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
