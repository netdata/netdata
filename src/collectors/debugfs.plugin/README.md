# OS provided metrics (debugfs.plugin)

`debugfs.plugin` gathers metrics from the `/sys/kernel/debug` folder on Linux
systems. [Debugfs](https://docs.kernel.org/filesystems/debugfs.html) exists as an easy way for kernel developers to
make information available to user space.

This plugin
is [external](https://github.com/netdata/netdata/tree/master/src/collectors#collector-architecture-and-terminology),
the netdata daemon spawns it as a long-running independent process.

In detail, it collects metrics from:

- `/sys/kernel/debug/extfrag` (Memory fragmentation index for each order and zone).
- `/sys/kernel/debug/zswap` ([Zswap](https://www.kernel.org/doc/Documentation/vm/zswap.txt) performance statistics).

## Prerequisites

### Permissions

> No user action required.

The debugfs root directory is accessible only to the root user by default. Netdata
uses [Linux Capabilities](https://man7.org/linux/man-pages/man7/capabilities.7.html) to give the plugin access
to debugfs. `CAP_DAC_READ_SEARCH` is added automatically during installation. This capability allows bypassing file read
permission checks and directory read and execute permission checks. If file capabilities are not usable, then the plugin is instead installed with the SUID bit set in permissions so that it runs as root.

## Metrics

| Metric                              |   Scope   |                                       Dimensions                                        |    Units     |  Labels   |
|-------------------------------------|:---------:|:---------------------------------------------------------------------------------------:|:------------:|:---------:|
| mem.fragmentation_index_dma         | numa node | order0, order1, order2, order3, order4, order5, order6, order7, order8, order9, order10 |    index     | numa_node |
| mem.fragmentation_index_dma32       | numa node | order0, order1, order2, order3, order4, order5, order6, order7, order8, order9, order10 |    index     | numa_node |
| mem.fragmentation_index_normal      | numa node | order0, order1, order2, order3, order4, order5, order6, order7, order8, order9, order10 |    index     | numa_node |
| system.zswap_pool_compression_ratio |           |                                    compression_ratio                                    |    ratio     |           |
| system.zswap_pool_compressed_size   |           |                                     compressed_size                                     |    bytes     |           |
| system.zswap_pool_raw_size          |           |                                    uncompressed_size                                    |    bytes     |           |
| system.zswap_rejections             |           |                 compress_poor, kmemcache_fail, alloc_fail, reclaim_fail                 | rejections/s |           |
| system.zswap_pool_limit_hit         |           |                                          limit                                          |   events/s   |           |
| system.zswap_written_back_raw_bytes |           |                                      written_back                                       |   bytes/s    |           |
| system.zswap_same_filled_raw_size   |           |                                       same_filled                                       |    bytes     |           |
| system.zswap_duplicate_entry        |           |                                         entries                                         |  entries/s   |           |

## Troubleshooting

To troubleshoot issues with the collector, run the `debugfs.plugin` in the terminal. The output
should give you clues as to why the collector isn't working.

- Navigate to the `plugins.d` directory, usually at `/usr/libexec/netdata/plugins.d/`. If that's not the case on
  your system, open `netdata.conf` and look for the `plugins` setting under `[directories]`.

  ```bash
  cd /usr/libexec/netdata/plugins.d/
  ```

- Switch to the `netdata` user.

  ```bash
  sudo -u netdata -s
  ```

- Run the `debugfs.plugin` to debug the collector:

  ```bash
  ./debugfs.plugin
  ```
