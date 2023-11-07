# bind_rndc_stats_file_size

**DNS | BIND**

BIND keeps track of statistics/metrics in a `*.stats` file, by the `statistics-file` option. The Netdata Agent monitors the size of this file. Receiving this alert means that the file exceeded a certain threshold.

This alert is triggered in warning state when the size of the file is greater than 512 MB and in critical state when it is greater than 1024 MB.

![](https://drive.google.com/uc?export=view&id=1elXR92OQn3sWVGXUCjpGi-NwcLNYE24g)

