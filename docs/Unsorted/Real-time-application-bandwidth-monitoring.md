# this is incomplete!

# Overview

Per application bandwidth monitoring is Linux, is problematic. The Linux kernel does not expose counters for the bandwidth its processes send or receive.

So how can we get bandwidth charts like this?

![qos](https://cloud.githubusercontent.com/assets/2662304/14389770/cd0a7c78-fdbc-11e5-9bcc-cc25bd0c4f42.gif)

The solution is very simple:

1. Use FireQOS to classify traffic for each application, at the network interface level
2. Use Netdata to visualize them in real-time

You can install these packages, on every server - and / or a Linux router that all the traffic passed through. Usually I do both. I install a basic FireQOS configuration on each server (no need for actual traffic shaping - just classification) and a more complex FireQOS configuration at the core routers of my DMZ.

Let's see an example:

## FireQOS

FireQOS is part of [FireHOL](http://firehol.org). If you install the firehol package using your linux distribution package manager, you will most probably get FireQOS too. Otherwise, you can follow [this procedure](https://github.com/firehol/firehol/wiki/Install-the-whole-firehol-suite) to install all the FireHOL tools directly from source.

FireQOS will setup `tc` classes and filters. Everything is handled by the Linux kernel, so you don't need to run a daemon. You configure it, you run it just once, and this is it.

You need a FireQoS configuration file (`/etc/firehol/fireqos.conf`) like this:

```

interface 

```

You can apply it using the command:

```sh
fireqos start
```

If you need more help to set it up, you can also follow the [FireQoS tutorial](https://github.com/firehol/firehol/wiki/FireQOS-Tutorial).

