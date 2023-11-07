# vernemq_average_scheduler_utilization

**Messaging | VerneMQ**

VerneMQ is implemented in Erlang and therefore runs on top of the BEAM runtime environment (roughly equivalent to
the JRE for Java applications).
For performance reasons, BEAM utilizes it’s own intenral scheduler that operates largely independently of the operating
system’s process scheduling.

The Netdata Agent calculates the average VerneMQ's scheduler utilization over the last 10 minutes.
This alert indicates high scheduler utilization.

This alert is raised into warning when the scheduler's utilization is between 75-85% and in critical
when it is between 85-95%.

### Troubleshooting section:

<details>
<summary>Check for CPU throttling issues </summary>

If you are receiving this alert often, it means that your node is running at maximum CPU utilization.
You should consider upgrading your system (instance in your cloud) to provide more or faster CPUs.

**Important**:

By default, the VerneMQ broker deploys its Erlang VM architecture into 4 cores. If you already
run VerneMQ in a multicore machine (for example, an 8-core machine) you should consider changing
the `vmq_bcrypt.nif_pool_size` parameter:

1. In the `vernemq.conf`, update the `vmq_bcrypt.nif_pool_size` parameter to `auto`. The value `auto`
detect all cores (n) and set the value to n-1. 


2. Restart the VerneMQ service.

   ```
   root@netdata # systemctl restart vernemq.service
   ```

3. Open the Netdata dashboard, and locate the `scheduler_utilization` chart. See if VerneMQ utilizes
   the preferred number of cores.

</details>