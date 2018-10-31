# Troubleshooting Alarms

Edit your netdata.conf and set `debug flags = 0x00800000`. Then check your `/var/log/netdata/debug.log`. It will show you how it works. Important: this will generate a lot of output in debug.log.

You can find the context of charts by looking up the chart in either `http://your.netdata:19999/netdata.conf` or `http://your.netdata:19999/api/v1/charts`.

You can find how netdata interpreted the expressions by examining the alarm at `http://your.netdata:19999/api/v1/alarms?all`. For each expression, netdata will return the expression as given in its config file, and the same expression with additional parentheses added to indicate the evaluation flow of the expression. 
