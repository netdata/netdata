### Understand the alert

This alert indicates that your system's Process ID (PID) space utilization is at high levels, meaning that there is a limited number of PIDs available for new processes. A warning state occurs when the percentage of used PIDs is between 85-90%, and a critical state occurs when it is between 90-95%. If the value reaches 100%, no new processes can be started.

### Troubleshoot the alert

1. **Identify high PID usage**: Use the `top` or `htop` command to identify processes with high PID usage. These processes may be causing the high PID space utilization.

2. **Check for zombie processes**: Zombie processes are processes that have completed execution but still occupy a PID, leading to high PID space utilization. Use the `ps axo stat,ppid,pid,comm | grep -w defunct` command to identify zombie processes. If you find any, investigate their parent processes and, if necessary, restart or terminate them to release the occupied PIDs.

3. **Monitor PID usage**: Continuously monitor your system's PID usage to understand normal behavior and identify potential issues before they become critical. You can use tools like Netdata for real-time monitoring.

4. **Adjust PID limits**: If your system consistently experiences high PID space utilization, consider increasing the maximum number of PIDs allowed. On Linux systems, you can adjust the `kernel.pid_max` sysctl parameter. Make sure to set this value according to your system's capacity and workload requirements.

5. **Optimize system performance**: Evaluate your system's workload and identify any specific processes or applications that are causing high PID usage. Optimize or limit these processes if necessary. Additionally, review your system's resource allocation and ensure there is sufficient capacity for process execution.

