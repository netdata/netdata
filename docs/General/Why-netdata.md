![image8](https://cloud.githubusercontent.com/assets/2662304/14253735/536f4580-fa95-11e5-9f7b-99112b31a5d7.gif)

# Netdata is unique!

## Per second data collection and visualization

**Per second data collection and visualization** is usually only available in dedicated console tools, like `top`, `vmstat`, `iostat`, etc. Netdata brings per second data collection and visualization to all applications, accessible through the web.

*You are not convinced per second data collection is important?*
**Click** this image for a demo:

[![image](https://cloud.githubusercontent.com/assets/2662304/12373555/abd56f04-bc85-11e5-9fa1-10aa3a4b648b.png)](http://netdata.firehol.org/demo2.html)

## Realtime

When netdata runs on modern computers (even on CELERON processors), most chart queries are replied in less than 3 milliseconds! **Not seconds, MILLISECONDS!** Less than 3 milliseconds for calculating the chart, generating JSON text, compressing it and sending it to your web browser. Timings are logged in netdata's `access.log` for you to examine.

Netdata is written in plain `C` and the key system plugins are written in `C` too. Its speed can only be compared to the native console system administration tools.

You can also stress test your netdata installation by running the script `tests/stress.sh` found in the distribution. Most modern server hardware can serve more than 300 chart refreshes per second per core. A raspberry pi 2, can serve 300+ chart refreshes per second utilizing all of its 4 cores.

## No disk I/O at all

Netdata does not use any disk I/O, apart its logs and even these can be disabled.

Netdata will use some memory (you size it, check [[Memory Requirements]]) and CPU (below 2% of a single core for the daemon, plugins may require more, check [[Performance]]), but normally your systems should have plenty of these resources available and spare.

The design goal of **NO DISK I/O AT ALL** effectively means netdata will not disrupt your applications.

## No root access

You don't need to run netdata as root. If started as root, netdata will switch to the `netdata` user (or any other user given in its configuration or command line argument).

There are a few plugins that in order to collect values need root access. These (and only these) are setuid to root.

## Embedded web server

No need to run something else to access netdata. Of course you can use a firewall, or a reverse proxy, to limit access to it. But for most systems, inside your DMZ, just running it will be enough.

## Configuration-less

Netdata supports plenty of [[Configuration]]. Though, we have done the most to allow netdata auto-detect most if not everything.

Even netdata plugins are designed to support configuration-less operation. So, you just install and run netdata. You will need to configure something only if it cannot be auto-detected.

## Visualizes QoS

Netdata visualizes `tc` QoS classes automatically. If you also use FireQOS, it will also collect interface and class names.

Check this animated GIF (generated with [ScreenToGif](https://screentogif.codeplex.com/)):

![animation5](https://cloud.githubusercontent.com/assets/2662304/12373715/0da509d8-bc8b-11e5-85cf-39d5234bf976.gif)

