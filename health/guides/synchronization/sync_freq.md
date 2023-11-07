# sync_freq

## OS: Any

By default, the Linux kernel writes data to disk asynchronously. Writes are buffered (cached) in 
memory, and written to the storage device at the optimal time.

Whenever you issue a write or send syscall or write to file-backed mappings or similar things, the
kernel is not forced to flush that data straight to persistent storage, the underlying network stack,
or any other subsystem. This buffering is implemented by the kernel for performance reasons.

The `sync()` system call writes any data buffered in memory out to disk. This can include (but is not limited to)
modified superblocks, modified inodes, and delayed reads and writes.

The Netdata Agent monitors the number of sync() system calls. Receiving this alert indicates a high
number of sync() system calls. Every call is very expensive because it causes all pending
modifications to filesystem metadata and cached file data to be written to the underlying
filesystems.

This alert is triggered in warning state when the number of sync() system calls is greater than 6.

<details>
   <summary>References and source </summary>
   
   1. [sync man pages](https://man7.org/linux/man-pages/man2/sync.2.html)
</details>

### Troubleshooting section

The `sync()` is expected to occur when the system is about to become unstable, or a storage device to become 
suddenly unavailable, and you want to ensure all data is written to disk. If you receive this alert often, you
should gather more information on why this event is happening.

<details>
   <summary>Use bpftrace to identify which process is causing these sync events</summary>
   
   `bpftrace` is a high-level tracing language for Linux enhanced Berkeley Packet Filter (eBPF) available in recent
   Linux kernels (4.x). bpftrace uses LLVM as a backend to compile scripts to BPF-bytecode and makes use of BCC 
   for interacting with the Linux BPF system, as well as existing Linux tracing capabilities such as kernel dynamic 
   tracing (kprobes), user-level dynamic tracing (uprobes), and tracepoints.
   
   One of the builtin tools in the `bpftrace` is the [syncsnoop](https://github.com/iovisor/bpftrace/blob/master/tools/syncsnoop_example.txt)
   which tracing the `sync` events
   
</details>
   

</details>
