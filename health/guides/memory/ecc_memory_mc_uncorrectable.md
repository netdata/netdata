# ecc_memory_mc_uncorrectable

## OS: Linux

Error correction code memory (ECC memory) is a type of computer data storage that uses an error
correction code (ECC) to detect and correct n-bit data corruption which occurs in memory. Error
correction codes protect against undetected memory data corruption, and is used in computers where
such corruption is unacceptable, for example in some scientific and financial computing
applications, or in database and file 
servers. <sup>[1](https://en.wikipedia.org/wiki/ECC_memory) </sup> 

The Netdata Agent monitors the number of ECC uncorrectable errors in the last 10 minutes.


<details>
<summary>See more on uncorrectable errors.</summary>

There are two main categories of Uncorrectable Errors (UE) as documented in the 
kernel.org <sup>[2](https://www.kernel.org/doc/Documentation/admin-guide/ras.rst) </sup> 

1. Fatal Error, when a UE error happens on a critical component of the system (for example, a piece
   of the Kernel got corrupted by a UE). The only reliable way to avoid data corruption is to hang
   or reboot the machine.

1. Non-fatal Error, when a UE error happens on an unused component, like an unused memory bank. The
   system may still run, eventually replacing the affected hardware by a hot spare, if available.

</details>


<details>
<summary>See more on machine checks</summary>

> Machine checks report internal hardware error conditions detected by the CPU. Uncorrected errors
typically cause a machine check (often with panic), corrected ones cause a machine check log entry.
>
> The behavior your machine will have when UE occurs depends on the tolerance level settings. The
tolerance level configures how hard the kernel tries to recover even at some risk of deadlock.
Higher tolerant values trade potentially better uptime with the risk of a crash or even corruption (
for tolerant >= 3). The Default is 1.
>
> - 0: always panic on uncorrected errors, log corrected errors
> 
> - 1: panic or SIGBUS on uncorrected errors, log corrected errors
> 
> - 2: SIGBUS or log uncorrected errors, log corrected errors
> 
> - 3: never panic or SIGBUS, log all errors (for testing 
> only) 
> 
> Also, when an error happens on a userspace process, it is also possible to kill such process and 
> let userspace restart it. <sup>[3](https://www.kernel.org/doc/html/v5.15-rc6/x86/x86_64/machinecheck.html) </sup>


</details>

<details>
<summary>References and sources:</summary>

1. [ECC memory on wikipedia](https://en.wikipedia.org/wiki/ECC_memory)

1. [Reliability, Availability and Serviceability concepts](https://www.kernel.org/doc/Documentation/admin-guide/ras.rst)
   
1. [Machine checks](https://www.kernel.org/doc/html/v5.2/x86/x86_64/machinecheck.html)

1. [memtester homepage](https://pyropus.ca/software/memtester/)

</details>

### Troubleshooting section:

<details>
<summary>Verify a bad memory module</summary>

Most of the times, uncorrectable errors will make your system and reboot/shutdown in a state of panic. If
not, that means that your tolerance level is high enough to not make the system go into panic. You
must identify the defective module immediately.

1. `memtester` is a userspace utility for testing the memory subsystem for faults. It's portable and 
   should compile and work on any 32 or 64-bit Unix-like system. For hardware developers, memtester
   can be told to test memory starting at a particular physical address (memtester v4.1.0+).
   <sup>[2](https://pyropus.ca/software/memtester/)

You may also receive this error as a result of incorrect seating or improper contact between the socket and
RAM module. Check on both before consider replacing the RAM module.
</details>

<details>
<summary>Check for BIOS updates</summary>

You should check for critical BIOS updates on your hardware's vendor support page.

</details>
