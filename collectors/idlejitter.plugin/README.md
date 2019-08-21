# idlejitter.plugin

It works like this:

A thread is spawned that requests to sleep for 20000 microseconds (20ms).
When the system wakes it up, it measures how many microseconds have passed.
The difference between the requested and the actual duration of the sleep, is the idle jitter.
This is done at most 50 times per second, to ensure we have a good average. 

This number is useful:

1.  in real-time environments, when the CPU jitter can affect the quality of the service (like VoIP media gateways).
2.  in cloud infrastructure, at can pause the VM or container for a small duration to perform operations at the host. 

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fcollectors%2Fidlejitter.plugin%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
