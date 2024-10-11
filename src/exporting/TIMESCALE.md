# Writing metrics to TimescaleDB

Thanks to Netdata's community of developers and system administrators, and Mahlon Smith
([GitHub](https://github.com/mahlonsmith)/[Website](http://www.martini.nu/)) in particular, Netdata now supports
archiving metrics directly to TimescaleDB.

What's TimescaleDB? Here's how their team defines the project on their [GitHub page](https://github.com/timescale/timescaledb):

> TimescaleDB is an open-source database designed to make SQL scalable for time-series data. It is engineered up from
> PostgreSQL, providing automatic partitioning across time and space (partitioning key), as well as full SQL support.

## Quickstart

To get started archiving metrics to TimescaleDB right away, check out Mahlon's [`netdata-timescale-relay`
repository](https://github.com/mahlonsmith/netdata-timescale-relay) on GitHub. Please be aware that backends subsystem
was removed and Netdata configuration should be moved to the new `exporting.conf` configuration file. Use

```text
[json:my_instance]
```

in `exporting.conf` instead of

```text
[backend]
    type = json
```

in `netdata.conf`.

This small program takes JSON streams from a Netdata client and writes them to a PostgreSQL (aka TimescaleDB) table.
You'll run this program in parallel with Netdata, and after a short [configuration
process](https://github.com/mahlonsmith/netdata-timescale-relay#configuration), your metrics should start populating
TimescaleDB.

Finally, another member of Netdata's community has built a project that quickly launches Netdata, TimescaleDB, and
Grafana in easy-to-manage Docker containers. Rune Juhl Jacobsen's
[project](https://github.com/runejuhl/grafana-timescaledb) uses a `Makefile` to create everything, which makes it
perfect for testing and experimentation.

## Netdata&#8596;TimescaleDB in action

Aside from creating incredible contributions to Netdata, Mahlon works at [LAIKA](https://www.laika.com/), an
Oregon-based animation studio that's helped create acclaimed films like _Coraline_ and _Kubo and the Two Strings_.

As part of his work to maintain the company's infrastructure of render farms, workstations, and virtual machines, he's
using Netdata, `netdata-timescale-relay`, and TimescaleDB to store Netdata metrics alongside other data from other
sources.

> LAIKA is a long-time PostgreSQL user and added TimescaleDB to their infrastructure in 2018 to help manage and store
> their IT metrics and time-series data. So far, the tool has been in production at LAIKA for over a year and helps them
> with their use case of time-based logging, where they record over 8 million metrics an hour for netdata content alone.

By archiving Netdata metrics to a database like TimescaleDB, LAIKA can consolidate metrics data from distributed
machines efficiently. Mahlon can then correlate Netdata metrics with other sources directly in TimescaleDB.

And, because LAIKA will soon be storing years worth of Netdata metrics data in TimescaleDB, they can analyze long-term
metrics as their films move from concept to final cut.

Read the full blog post from LAIKA at the [TimescaleDB
blog](https://blog.timescale.com/blog/writing-it-metrics-from-netdata-to-timescaledb/amp/).

Thank you to Mahlon, Rune, TimescaleDB, and the members of the Netdata community that requested and then built this
exporting connection between Netdata and TimescaleDB!
