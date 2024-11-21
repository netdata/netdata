### Understand the alert

Your system communicates with the devices attached to it through interrupt requests. In a nutshell, when an interrupt occurs, the operating system stops what it was doing and starts addressing that interrupt.

Network interfaces can receive thousands of packets per second. To avoid burying the system with thousands of interrupts, the Linux kernel uses the NAPI polling framework. In this way, we can replace hundreds of hardware interrupts with one poll by managing them with a few Soft Interrupt ReQuests (Soft IRQs). Ksoftirqd is a per-CPU kernel thread responsible for handling those unserved Soft Interrupt ReQuests (Soft IRQs). The Netdata Agent inspects the average number of times Ksoftirqd ran out of netdev_budget or CPU time when there was still work to be done. This abnormality may cause packet overflow on the intermediate buffers and, as a result, drop packet on your network interfaces.

The default value of the netdev_budget is 300.  However, this may not be enough in some cases, such as:

- Multiple interfaces operating at 1Gbps, or even a single interface at 10Gbps.

- Lower powered systems processing very large amounts of network traffic.

### NAPI polling mechanism. 

The design of NAPI allows the network driver to go into a polling mode, buffering the packets it receives into a ring-buffer, and raises a soft interrupt to start a NAPI polling cycle instead of being hard-interrupted for 
every packet. Linux kernel through NAPI will poll data from the buffer until the netdev_budget_usecs times out or the number of packets reaches the netdev_budget limit.

- netdev_budget_usecs variable defines the maximum number of microseconds in one NAPI polling cycle.
- netdev_budget variable defines the maximum number of packets taken from all interfaces in one polling cycle.

### Troubleshoot the alert

- Increase the netdev_budget value.

1. Check your current value.
   
    ```
    root@netdata~ $ sysctl net.core.netdev_budget
    net.core.netdev_budget = 300
    ```
   
2. Try to increase it gradually with increments of 100.	

    ```
    root@netdata~ $ sysctl -w net.core.netdev_budget=400
    ``` 

3. Verify the change and test it with the same workload that triggered the alarm originally. If the problem still exists, try to 
   increment it again.
   
    ```
    root@netdata~ $ sysctl net.core.netdev_budget
    net.core.netdev_budget = 400
    ```
 
4. If this change works for your system, you could make it permanently.
   
   Bump this `net.core.netdev_budget=<desired_value>` entry under `/etc/sysctl.conf`


5. Reload the sysctl settings.
   
    ```
    root@netdata~ $ sysctl -p
    ```