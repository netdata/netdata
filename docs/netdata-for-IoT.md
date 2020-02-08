# Netdata for IoT

![image1](https://cloud.githubusercontent.com/assets/2662304/14252446/11ae13c4-fa90-11e5-9d03-d93a3eb3317a.gif)

> New to Netdata? Check its demo: **<https://my-netdata.io/>**
>
>[![User
>Base](https://registry.my-netdata.io/api/v1/badge.svg?chart=netdata.registry_entries&dimensions=persons&label=user%20base&units=null&value_color=blue&precision=0&v41)](https://registry.my-netdata.io/#netdata_registry)
>[![Monitored
>Servers](https://registry.my-netdata.io/api/v1/badge.svg?chart=netdata.registry_entries&dimensions=machines&label=servers%20monitored&units=null&value_color=orange&precision=0&v41)](https://registry.my-netdata.io/#netdata_registry)
>[![Sessions
>Served](https://registry.my-netdata.io/api/v1/badge.svg?chart=netdata.registry_sessions&label=sessions%20served&units=null&value_color=yellowgreen&precision=0&v41)](https://registry.my-netdata.io/#netdata_registry)
>
>[![New Users
>Today](https://registry.my-netdata.io/api/v1/badge.svg?chart=netdata.registry_entries&dimensions=persons&after=-86400&options=unaligned&group=incremental-sum&label=new%20users%20today&units=null&value_color=blue&precision=0&v40)](https://registry.my-netdata.io/#netdata_registry)
>[![New Machines
>Today](https://registry.my-netdata.io/api/v1/badge.svg?chart=netdata.registry_entries&dimensions=machines&group=incremental-sum&after=-86400&options=unaligned&label=servers%20added%20today&units=null&value_color=orange&precision=0&v40)](https://registry.my-netdata.io/#netdata_registry)
>[![Sessions
>Today](https://registry.my-netdata.io/api/v1/badge.svg?chart=netdata.registry_sessions&after=-86400&group=incremental-sum&options=unaligned&label=sessions%20served%20today&units=null&value_color=yellowgreen&precision=0&v40)](https://registry.my-netdata.io/#netdata_registry)

---

Netdata is a **very efficient** server performance monitoring solution. When running in server hardware, it can collect
thousands of system and application metrics **per second** with just 1% CPU utilization of a single core. Its web server
responds to most data requests in about **half a millisecond** making its web dashboards spontaneous, amazingly fast!

Netdata can also be a very efficient real-time monitoring solution for **IoT devices** (RPIs, routers, media players,
wifi access points, industrial controllers and sensors of all kinds). Netdata will generally run everywhere a Linux
kernel runs (and it is glibc and [musl-libc](https://www.musl-libc.org/) friendly).

You can use it as both a data collection agent (where you pull data using its API), for embedding its charts on other
web pages / consoles, but also for accessing it directly with your browser to view its dashboard.

The Netdata web API already provides **reduce** functions allowing it to report **average** and **max** for any
timeframe. It can also respond in many formats including JSON, JSONP, CSV, HTML. Its API is also a **google charts**
provider so it can directly be used by google sheets, google charts, google widgets.

![sensors](https://cloud.githubusercontent.com/assets/2662304/15339745/8be84540-1c8e-11e6-9e9a-106dea7539b6.gif)

Although Netdata has been significantly optimized to lower the CPU and RAM resources it consumes, the plethora of data
collection plugins may be inappropriate for weak IoT devices. Please follow the guide on [running Netdata in embedded
devices](Performance.md)

## Monitoring RPi temperature

The python version of the sensors plugin uses `lm-sensors`. Unfortunately the temperature reading of RPi are not
supported by `lm-sensors`.

Netdata also has a bash version of the sensors plugin that can read RPi temperatures. It is disabled by default to avoid
the conflicts with the python version.

To enable it, run `sudo edit-config charts.d.conf` and uncomment this line:

```sh
sensors=force
```

Then restart Netdata. You will get this:

![image](https://user-images.githubusercontent.com/2662304/29658868-23aa65ae-88c5-11e7-9dad-c159600db5cc.png)

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fdocs%2Fnetdata-for-IoT&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
