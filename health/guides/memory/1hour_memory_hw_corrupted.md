
### Understand the alert
The Linux kernel keeps track of the system memory state. You can find the actual values it tracks in the [man pages](https://man7.org/linux/man-pages/man5/proc.5.html) under the `/proc/meminfo` subsection. One of the values that the kernel reports is the `HardwareCorrupted` , which is the amount of memory, in kibibytes (1024 bytes), with physical memory corruption problems, identified by the hardware and set aside by the kernel so it does not get used.

The Netdata Agent monitors this value. This alert indicates that the memory is corrupted due to a hardware failure. While primarily the error may be due to a failing RAM chip, it can also be caused by incorrect seating or improper contact between the socket and memory module.

### Troubleshoot the alert

Most of the time uncorrectable errors will make your system reboot/shutdown in a state of panic. If not, that means that your tolerance level is high enough to not make the system go into panic. You must identify the defective module immediately.

`memtester` is a userspace utility for testing the memory subsystem for faults. 

You may also receive this error as a result of incorrect seating or improper contact between the socket and RAM module. Check both before consider replacing the RAM module.

### Useful resources

1. [man pages /proc](https://man7.org/linux/man-pages/man5/proc.5.html)
2. [memtester homepage](https://pyropus.ca/software/memtester/)

