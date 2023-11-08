### Understand the alert

The `vsphere_cpu_usage` alert monitors the average CPU utilization of virtual machines in the vSphere platform. The alert is triggered in a warning state when the CPU utilization is between 75-85% and in a critical state when it is between 85-95%.

### What does high CPU usage mean?

High CPU usage indicates that the virtual machine's CPU resources are being heavily utilized. This can lead to performance issues, slow response times, and decreased stability.

### Troubleshoot the alert

1. Confirm the high CPU usage by logging into the vSphere management console and checking the CPU performance metrics for the affected virtual machine(s).

2. Identify the cause of high CPU usage:

   - Check the virtual machine's running processes to identify any resource-intensive applications or services. You can use the `top` command on Linux-based virtual machines or Task Manager on Windows-based virtual machines.
   - Inspect application logs and system logs for any signs of issues, errors, or crashes that could be contributing to high CPU usage.
   - Verify if the virtual machine has adequate CPU resources allocated. If the virtual machine is consistently using a high percentage of its allocated CPU resources, consider increasing the allocated CPU resources.

3. Remediate the issue:

   - If an application or service is responsible for the high CPU usage, try restarting it or addressing the specific issue causing the problem.
   - If the virtual machine is consistently using a high percentage of its allocated CPU resources, consider increasing the allocated CPU resources or optimizing the virtual machine's performance through application and OS tuning.
   - Monitor the CPU usage after making changes to ensure that the issue has been resolved.

### Useful resources

1. [vSphere Monitoring and Performance Guide](https://docs.vmware.com/en/VMware-vSphere/7.0/com.vmware.vsphere.monitoring.doc/GUID-0C94837C-8CA4-4A4E-9694-FE9828979A77.html)
2. [Identifying and Troubleshooting CPU Performance Issues in VMware](https://kb.vmware.com/s/article/2090599)
3. [Optimizing Performance on Hyper-V and VMware Virtual Machines](https://info.raindanceit.com/blog/optimizing-performance-hyper-v-vmware)