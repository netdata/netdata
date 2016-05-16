# Memory De-duplication is Kernel Same-Page Merging

Netdata offers all its round robin database to kernel for deduplication.

## Enable KSM in kernel

You need to run a kernel compiled with:

```sh
CONFIG_KSM=y
```

When KSM is enabled at the kernel is just available for the user to enable it.

So, if you build a kernel with `CONFIG_KSM=y` you will just get a few files in `/sys/kernel/mm/ksm`. Nothing else happens. There is no performance penalty (apart I guess from the memory this code occupies into the kernel).

The files that `CONFIG_KSM=y` offers include:

- `/sys/kernel/mm/ksm/run` by default `0`. You have to set this to `1` for the kernel to spawn `ksmd`.
- `/sys/kernel/mm/ksm/sleep_millisecs`, by default `20`. The frequency ksmd should evaluate memory for deduplication.
- `/sys/kernel/mm/ksm/pages_to_scan`, by default `100`. The amount of pages ksmd will evaluate on each run.

So, by default `ksmd` is just disabled. It will not harm performance and the user/admin can control the CPU resources he/she is willing `ksmd` to use.

## Run `ksmd` kernel daemon

To activate / run `ksmd` you need to run:

```sh
echo 1 >/sys/kernel/mm/ksm/run
echo 1000 >/sys/kernel/mm/ksm/sleep_millisecs
```

With these settings ksmd does not even appear in the running process list (it will run once per second and evaluate 100 pages for de-duplication).

Put the above lines in your boot sequence (`/etc/rc.local` or equivalent) to have `ksmd` run at boot.

## Monitoring Kernel Memory de-duplication performance

Netdata will create charts for kernel memory de-duplication performance, like this:

![image](https://cloud.githubusercontent.com/assets/2662304/11998786/eb23ae54-aab6-11e5-94d4-e848e8a5c56a.png)

