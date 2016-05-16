Live demo of QoS: **[see QoS in action here](http://netdata.firehol.org/#tc)** !

![image1](https://cloud.githubusercontent.com/assets/2662304/14252446/11ae13c4-fa90-11e5-9d03-d93a3eb3317a.gif)

---

# You should install QoS on all your servers

I hear a lot of you complaining already!

`tc` (the command that configures QoS in Linux) is probably **the most undocumented, complicated and unfriendly** command in Linux.

Let's see an example: To match a simple port range in `tc`, all the high ports from 1025 to 65535 inclusive, we have to match these:

```
1025/0xffff 1026/0xfffe 1028/0xfffc 1032/0xfff8 1040/0xfff0
1056/0xffe0 1088/0xffc0 1152/0xff80 1280/0xff00 1536/0xfe00
2048/0xf800 4096/0xf000 8192/0xe000 16384/0xc000 32768/0x8000
```

I know what you are thinking right now! **And I agree!** 

Don't leave yet, **You will not have to do this**!

Before presenting the solution though, let's see what is QoS and why we need it.

---

## QoS

QoS is about 2 features:

1. **Traffic Classification**

  Classification is the process of organizing traffic in groups, called **classes**. Classification can evaluate every aspect of network packets, like source and destination ports, source and destination IPs, netfilter marks, etc.

  To simplify things, you can think that when you classify traffic, you just assign a label to it. Of course classes have some properties themselves (like queuing mechanisms), but let's say it is that simple: **a label**.

2. **Apply traffic shaping rules to these classes**

  Traffic shaping is used to control how network bandwidth should be shared among the classes. Normally, you need to do this, when there is not enough bandwidth to satisfy all the demand, or when you want to control the supply of bandwidth to certain services.

So, why do we need it?

---

## We need QoS

I install QoS for the following reasons:

1. **Monitoring the bandwidth used by services**

  netdata provides wonderful real-time charts, like this one (wait to see the orange `rsync` part):

 ![qos3](https://cloud.githubusercontent.com/assets/2662304/14474189/713ede84-0104-11e6-8c9c-8dca5c2abd63.gif)

2. **Ensure sensitive administrative tasks will not starve for bandwidth**

  Have you tried to ssh to a server when the network is congested? If you have, you already know it does not work very well. QoS can guarantee that services like ssh, dns, ntp, etc will always have a small supply of bandwidth. So, no matter what happens, you will be able to ssh to your server and DNS will always work.

3. **Ensure administrative tasks will not monopolize all the bandwidth**

  Services like backups, file copies, database dumps, etc can easily monopolize all the available bandwidth. It is common for example a nightly backup, or a huge file transfer to negatively influence the end-user experience. QoS can fix that.

4. **Ensure each end-user connection will get a fair cut of the available bandwidth.**

  Several QoS queuing disciplines in Linux do this automatically, without any configuration from you. The result is that new sockets are favored over older ones, so that users will get a snappier experience, while others are transferring large amounts of traffic.

5. **Protect the servers from DDoS attacks.**

   This is my favorite. When your system is under a DDoS attack, it will get a lot more bandwidth compared to the one it can handle and probably your applications will crash. Setting a limit on the inbound traffic using QoS, will protect your servers (throttle the requests) and depending on the size of the attack may allow your legitimate users to access the server, while the attack is taking place.

  Using QoS together with a [SYNPROXY](https://github.com/firehol/netdata/wiki/Monitoring-SYNPROXY) will provide a great degree of protection against most DDoS attacks. Actually when I wrote that article, a few folks tried to DDoS the netdata demo site to see in real-time the SYNPROXY operation. They did not do it right, but anyway a great deal of requests reached the netdata server. What saved netdata was QoS. The netdata demo server has QoS installed, so the requests were throttled and the server did not even reach the point of resource starvation. Read about it [here](https://github.com/firehol/netdata/wiki/Monitoring-SYNPROXY#a-note-for-ddos-testers).

On top of all these, QoS is extremely light. You will configure it once, and this is it. It will not bother you again and it will not use any noticeable CPU resources, especially on application and database servers.

So, do we have to learn this hex mess?

---

## Setup QoS, the right way!

I wrote and use **[FireQOS](https://firehol.org/tutorial/fireqos-new-user/)**, a tool to simplify QoS management in Linux.

This is the FireQOS configuration file `/etc/firehol/fireqos.conf` we use at the netdata demo site:

```sh
    # configure the netdata ports
    server_netdata_ports="tcp/19999"

    interface eth0 world bidirectional ethernet balanced rate 50Mbit
       class arp
          match arp

       class icmp
          match icmp

       class dns commit 1Mbit
          server dns
          client dns

       class ntp
          server ntp
          client ntp

       class ssh commit 2Mbit
          server ssh
          client ssh

       class rsync commit 2Mbit max 10Mbit
          server rsync
          client rsync

       class web_server commit 40Mbit
          server http
          server netdata

       class client
          client surfing

       class nms commit 1Mbit
          match input src 10.2.3.5
```

Nothing more is needed. You edit `/etc/firehol/fireqos.conf` to configure your classes and their bandwidth limits, you run `fireqos start` to apply it and open your netdata dashboard to see real-time visualization of the bandwidth consumption of your applications. FireQOS is not a daemon. It will just convert the configuration to `tc` commands (and handle all the hex mess for you), it will run them and it will exit.

**IMPORTANT**: If you copy this configuration to apply it to your system, please adapt the speeds - experiment in non-production environments to learn the tool, before applying it on your servers.

---

## Install FireQOS

The **[FireHOL](https://firehol.org/)** package already distributes **[FireQOS](https://firehol.org/tutorial/fireqos-new-user/)**. Check the **[FireQOS tutorial](https://firehol.org/tutorial/fireqos-new-user/)** to learn how to write your own QoS configuration.

With **[FireQOS](https://firehol.org/tutorial/fireqos-new-user/)**, it is **really simple for everyone to use QoS in Linux**.

Just install the package `firehol`. It should already be available for your distribution. If not, check the **[FireHOL Installation Guide](https://firehol.org/installing/)**.

After installing FireHOL, you will have the `fireqos` command.

---

## More examples:

This is QoS from my home linux router. Check these features:

1. It is real-time (per second updates)
2. QoS really works in Linux - check that the `background` traffic is squeezed when `surfing` needs it.

![test2](https://cloud.githubusercontent.com/assets/2662304/14093004/68966020-f553-11e5-98fe-ffee2086fafd.gif)