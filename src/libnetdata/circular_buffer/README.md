<!--
title: "circular_buffer"
custom_edit_url: https://github.com/netdata/netdata/edit/master/libnetdata/circular_buffer/README.md
-->

# Circular Buffer

`struct circular_buffer` is an adaptive circular buffer. It will start at an initial size
and grow up to a maximum size as it fills. Two indices within the structure track the current
`read` and `write` position for data.

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Flibnetdata%2Fcircular_buffer%2README&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)]()
