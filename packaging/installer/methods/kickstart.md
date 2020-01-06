# Netdata installation via one-line installation script

![](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.requests_per_url&options=unaligned&dimensions=kickstart&group=sum&after=-3600&label=last+hour&units=installations&value_color=orange&precision=0) ![](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.requests_per_url&options=unaligned&dimensions=kickstart&group=sum&after=-86400&label=today&units=installations&precision=0)

This method is fully automatic on all Linux distributions. To install Netdata from source and get _automatic nightly
updates_, run the following:

```bash
bash <(curl -Ss https://my-netdata.io/kickstart.sh)
```



> Do not use `sudo` for the one-line installer—it will escalate privileges itself if needed.

Now that Netdata is installed, be sure to visit our getting started guide for a quick overview of configuring Netdata,
enabling plugins, and controlling Netdata's daemon. Or, get the full guided tour of Netdata's capabilities with our
step-by-step tutorial!