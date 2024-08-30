### Understand the alert

This alert calculates the average total `CPU utilization` on a Windows system over the last 10 minutes. If you receive this warning or critical alert, it means that your system is experiencing high CPU usage, which could lead to performance issues.

### What does CPU utilization mean?

`CPU utilization` is the percentage of time the CPU spends executing tasks, as opposed to being idle. A high CPU utilization means that the CPU is working on a large number of tasks and may not have enough processing power to handle additional tasks efficiently. This can result in slow response times and overall system performance issues.

### Troubleshoot the alert

1. Identify high CPU usage processes:
   
   Open Task Manager by pressing `Ctrl + Shift + Esc` on your keyboard, or right-click on the Taskbar and select "Task Manager." Click the "Processes" tab, and sort by the "CPU" column to identify the processes consuming the most CPU resources.

2. Analyze process details:
   
   Right-click on the process with high CPU usage and select "Properties" or "Go to details" to learn more about the process, its location, and its purpose.

3. Determine if the process is essential:
   
   Research the process in question to ensure that it is safe to terminate. Some processes are integral to the system, and terminating them may cause instability or crashes.

4. Terminate or optimize the problematic process:
   
   If the process is not essential, you can right-click on it and select "End task" to stop it. If the process is necessary, consider optimizing its performance or updating the software responsible for the process. In some cases, restarting the system may help resolve temporary high CPU usage issues.

5. Monitor CPU usage after taking action:
   
   Continue monitoring CPU usage to ensure that the issue has been resolved. If the problem persists, further investigation may be required, such as examining system logs or using performance analysis tools like Windows Performance Monitor.

### Useful resources

1. [Windows Task Manager: A Troubleshooting Guide](https://www.howtogeek.com/66622/stupid-geek-tricks-6-ways-to-open-windows-task-manager/)
2. [How to Use the Performance Monitor on Windows](https://www.digitalcitizen.life/how-use-performance-monitor-windows/)
3. [Understanding Process Explorer](https://docs.microsoft.com/en-us/sysinternals/downloads/process-explorer)