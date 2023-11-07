# ecc_memory_mc_correctable

## OS: Linux

*Error correction code memory (ECC memory) is a type of computer data storage that uses an error
correction code (ECC) to detect and correct n-bit data corruption which occurs in memory. ECC
memory is used in most computers where data corruption cannot be tolerated under any circumstances,
like industrial control applications, critical databases, and infrastructural memory 
caches.* <sup>[1](https://en.wikipedia.org/wiki/ECC_memory) </sup> 

"Correctable errors are generally single-bit errors that the system or the built-in ECC mechanism
can correct. These errors do not cause system downtime of data 
corruption." <sup>[2](https://www.atpinc.com/blog/ecc-dimm-memory-ram-errors-types-chipkill) </sup>

Netdata agent monitors the number of ECC correctable errors in the last 10 minutes.

<details>
<summary>References and sources:</summary>

1. [ECC memory on wikipedia](https://en.wikipedia.org/wiki/ECC_memory)

1. [RAM types and ECC technologies](https://www.atpinc.com/blog/ecc-dimm-memory-ram-errors-types-chipkill)

1. [memtester homepage](https://pyropus.ca/software/memtester/)

</details>

### Troubleshooting section:

<details>
<summary>Verify a bad memory module</summary>

Correctable errors do not necessarily indicate hardware failures, but should generally still be investigated.

1. `memtester` is a userspace utility for testing the memory subsystem for faults. It's portable and 
   should compile and work on any 32 or 64-bit Unix-like system. For hardware developers, memtester
   can be told to test memory starting at a particular physical address (memtester v4.1.0+).
   <sup>[3](https://pyropus.ca/software/memtester/)

You can also get this kind of errors by incorrect seating or improper contact between the socket and
RAM module. Check on both before consider replacing the RAM module.
</details>

<details>
<summary>Check for BIOS updates</summary>

You should check for critical BIOS updates on your hardware's vendor support page.

</details>
