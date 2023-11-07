# 10min_netisr_backlog_exceeded				

## OS: FreeBSD

The netisr_maxqlen is a queue within the network kernel dispatch service of FreeBSD kernel which keeps packets 
received by interfaces and not yet processed by destined subsystems or userland applications. The system drops new packets
when the queue is full. There may be several netisr packet queues in the system and raising netisr_maxqlen 
allows all of them to grow. The default netisr_maxqlen value should be 256 in most of the FreeBSD versions. 
 However this may not be enough in some cases, such as:

- Multiple interfaces operating at 1Gbps, or even a single interface at 10Gbps.
  
- Lower powered systems process very large amounts of network traffic.

Netdata agent monitors the average number of dropped packets in the last minute due to exceeded netisr queue length.

### Troubleshooting section:

 <details>
    <summary>Increase the netisr_maxqlen value.</summary>

1. Check your current value.
   
    ```
    root@netdata~ # sysctl net.route.netisr_maxqlen
    net.route.netisr_maxqlen: 256
    ```
   
2. Try to increase it by a factor of 4.	

    ```
    root@netdata~ # sysctl -w net.route.netisr_maxqlen=1024
    ``` 

3. Verify the change and test with the same workload that triggered the alarm originally.
   
    ```
    root@netdata~ # sysctl net.route.netisr_maxqlen
    net.route.netisr_maxqlen: 1024
    ```
 
4. If this change works for your system, you could make it permanently.
   
   Bump this `net.route.netisr_maxqlen=1024` entry under `/etc/sysctl.conf`


5. Reload the sysctl settings.
   
    ```
    root@netdata~ # /etc/rc.d/sysctl reload
    ```
</details>


