# 1hour_memory_hw_corrupted

## OS: Linux

The Linux kernel keeps track of the system memory state. You can find the actual values it tracks in
the man pages <sup>[1](https://man7.org/linux/man-pages/man5/proc.5.html) </sup>  under
the `/proc/meminfo` subsection. One of the values that the kernel reports is the `HardwareCorrupted`
, which is the amount of memory, in kibibytes (1024 bytes), with physical memory corruption
problems, identified by the hardware and set aside by the kernel so it does not get used.

The Netdata Agent monitors this value. This alert indicates that the memory is corrupted due to a
hardware failure. While primarily the error may be due to a failing RAM chip, it can also be caused
by incorrect seating or improper contact between the socket and memory module.

<details>
<summary>References and Sources</summary>

1. [man pages /proc](https://man7.org/linux/man-pages/man5/proc.5.html)

1. [memtester homepage](https://pyropus.ca/software/memtester/)

</details>

### Troubleshooting section:

<details>
<summary>Verify a bad memory module</summary>

Most of the times, uncorrectable errors will make your system and reboot/shutdown in a state of
panic. If not, that means that your tolerance level is high enough to not make the system go into
panic. You must identify the defective module immediately.

1. `memtester` is a userspace utility for testing the memory subsystem for faults. It's portable and 
   should compile and work on any 32 or 64-bit Unix-like system. For hardware developers, memtester
   can be told to test memory starting at a particular physical address (memtester v4.1.0+).
   <sup>[2](https://pyropus.ca/software/memtester/)

You may also receive this error as a result of incorrect seating or improper contact between the
socket and RAM module. Check on both before consider replacing the RAM module.
</details>
