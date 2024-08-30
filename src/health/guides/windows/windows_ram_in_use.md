### Understand the alert

The `windows_ram_in_use` alert is triggered when memory utilization on a Windows system reaches the specified warning or critical thresholds. If you receive this alert, it means that your Windows system is running low on available memory.

### What does memory utilization mean?

Memory utilization refers to the percentage of a system's RAM that is currently being used by applications, processes, and the operating system. High memory utilization can lead to performance issues and may cause applications to crash or become unresponsive.

### Troubleshoot the alert

- Check current memory usage on the system

1. Press `Ctrl + Shift + Esc` to open Task Manager.
2. Click on the `Performance` tab.
3. View the `Memory` section to see the total memory usage and available memory. 

- Identify high memory usage processes

1. In Task Manager, click on the `Processes` tab.
2. Click on the `Memory` column to sort processes by memory usage.
3. Identify processes that are using a high percentage of memory.

- Optimize memory usage

1. Close unnecessary applications and processes to free up memory.
2. Investigate if running processes have a known memory leak issue.
3. Consider upgrading the system's RAM if memory usage is consistently high.

- Monitor memory usage over time

1. Use Windows Performance Monitor to create a Data Collector Set that collects memory usage metrics.
2. Analyze the collected data to identify trends and potential issues.

### Useful resources

1. [How to use Performance Monitor on Windows 10](https://www.windowscentral.com/how-use-performance-monitor-windows-10)