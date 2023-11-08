### Understand the alert

The Netdata Agent checks if your system is in sync with a Network Time Protocol (NTP) server. This alert indicates that the system time is not synchronized to a reliable server. It is strongly recommended having the clock in sync with NTP servers, because, otherwise, it leads to unpredictable problems that are difficult to debug especially in matters of security.

### Troubleshoot the alert

Different linux distros utilize different NTP tools. You can always install `ntp`. If your clock is out of sync, you should first check for issues in your network connectivity. 
  
### Useful resources

[Best practices for NTP servers](https://bluecatnetworks.com/blog/seven-best-practices-to-keep-your-ntp-resilient/).