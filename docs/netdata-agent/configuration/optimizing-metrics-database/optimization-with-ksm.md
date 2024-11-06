# Database optimization with KSM

KSM performs memory deduplication by scanning through main memory for physical pages that have identical content, and
identifies the virtual pages that are mapped to those physical pages. It leaves one page unchanged, and re-maps each
duplicate page to point to the same physical page. Netdata offers all of its in-memory database to kernel for
deduplication.

In the past, KSM has been criticized for consuming a lot of CPU resources. This is true when KSM is used for
deduplicating certain applications, but it is not true for Netdata. Agent's memory is written very infrequently
(if you have 24 hours of metrics in Netdata, each byte at the in-memory database will be updated just once per day). KSM
is a solution that will provide 60+% memory savings to Netdata.

## Enable KSM in kernel

To enable KSM in kernel, you need to run a kernel compiled with the following:

```sh
CONFIG_KSM=y
```

When KSM is enabled at the kernel, it is just available for the user to enable it.

If you build a kernel with `CONFIG_KSM=y`, you will just get a few files in `/sys/kernel/mm/ksm`. Nothing else
happens. There is no performance penalty (apart from the memory this code occupies into the kernel).

The files that `CONFIG_KSM=y` offers include:

- `/sys/kernel/mm/ksm/run` by default `0`. You have to set this to `1` for the kernel to spawn `ksmd`.
- `/sys/kernel/mm/ksm/sleep_millisecs`, by default `20`. The frequency ksmd should evaluate memory for deduplication.
- `/sys/kernel/mm/ksm/pages_to_scan`, by default `100`. The amount of pages ksmd will evaluate on each run.

So, by default `ksmd` is just disabled. It will not harm performance and the user/admin can control the CPU resources
they are willing to have used by `ksmd`.

## Run `ksmd` kernel daemon

To activate / run `ksmd,` you need to run the following:

```sh
echo 1 >/sys/kernel/mm/ksm/run
echo 1000 >/sys/kernel/mm/ksm/sleep_millisecs
```

With these settings, ksmd does not even appear in the running process list (it will run once per second and evaluate 100
pages for de-duplication).

Put the above lines in your boot sequence (`/etc/rc.local` or equivalent) to have `ksmd` run at boot.

## Monitoring Kernel Memory de-duplication performance

Netdata will create charts for kernel memory de-duplication performance, the **deduper (ksm)** charts can be seen under the **Memory** section in the Netdata UI.

### KSM summary

The summary gives you a quick idea of how much savings (in terms of bytes and in terms of percentage) KSM is able to achieve.

![image](https://user-images.githubusercontent.com/24860547/199454880-123ae7c4-071a-4811-95b8-18cf4e4f60a2.png)

### KSM pages merge performance

This chart indicates the performance of page merging. **Shared** indicates used shared pages, **Unshared** indicates memory no longer shared (pages are unique but repeatedly checked for merging), **Sharing** indicates memory currently shared(how many more sites are sharing the pages, i.e. how much saved) and **Volatile** indicates volatile pages (changing too fast to be placed in a tree).

A high ratio of Sharing to Shared indicates good sharing, but a high ratio of Unshared to Sharing indicates wasted effort.

![image](https://user-images.githubusercontent.com/24860547/199455374-d63fd2c2-e12b-4ddf-947b-35371215eb05.png)

### KSM savings

This chart shows the amount of memory saved by KSM. **Savings** indicates saved memory. **Offered** indicates memory marked as mergeable.

![image](https://user-images.githubusercontent.com/24860547/199455604-43cd9248-1f6e-4c31-be56-e0b9e432f48a.png)

### KSM effectiveness

This chart tells you how well KSM is doing at what it is supposed to. It does this by charting the percentage of the mergeable pages that are currently merged.

![image](https://user-images.githubusercontent.com/24860547/199455770-4d7991ff-6b7e-4d96-9d23-33ffc572b370.png)
