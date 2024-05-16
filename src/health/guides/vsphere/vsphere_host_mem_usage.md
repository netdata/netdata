### Understand the alert

The `vsphere_host_mem_usage` alert is triggered when the memory utilization of a vSphere host reaches critical levels. This alert is raised to a warning level when the utilization exceeds 90% and becomes critical when it exceeds 98%. High memory utilization can lead to performance issues on the virtual machines running on the host.

### Troubleshoot the alert

1. Log in to the vSphere client:
   
   Access the vSphere client to get an overview of your host's memory utilization and to identify which virtual machines are consuming the most memory.

2. Identify high memory-consuming virtual machines:

   In the vSphere client, go to the "Hosts and Clusters" view and select the affected host. In the "Virtual Machines" tab, you can now see the memory usage of each virtual machine running on the host. Identify any virtual machines that are consuming a high amount of memory.

3. Analyze the memory usage in the virtual machines:

   Connect to the high memory-consuming virtual machines and use their respective task managers (e.g., "top" command in Linux or Task Manager in Windows) to identify the applications and processes that are causing the high memory usage.

4. Take action:

   - If an application or process is consuming an excessive amount of memory and is not required, consider stopping it.
   - Alternatively, if the application or process is essential, you may need to allocate more memory to the virtual machine or consider moving the workload to a different host with more available resources.
   - Ensure the virtual machine's memory is optimally configured, as over-allocating memory may cause contention.
   
5. Monitor the situation:

   Keep an eye on the memory utilization of the host and the virtual machines after making changes. If memory utilization remains high, consider analyzing other virtual machines or adding more memory to the host.

### Useful resources

1. [vSphere Monitoring and Performance Documentation](https://docs.vmware.com/en/VMware-vSphere/7.0/com.vmware.vsphere.monitoring.doc/GUID-115861E6-810A-43BB-8CDB-EE99CF8F3250.html)
2. [Optimizing Memory Performance in VMware vSphere](https://blogs.vmware.com/performance/2021/04/optimizing-memory-performance-in-vmware-vsphere.html)