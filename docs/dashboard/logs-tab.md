# Logs tab

The Logs tab is using the [`systemd` journal plugin](https://github.com/netdata/netdata/blob/master/src/collectors/systemd-journal.plugin/README.md), to present a structured view into your infrastructure's `systemd` logs.

We have a thorough section explaining how you can [work with logs](https://github.com/netdata/netdata/blob/master/docs/category-overview-pages/logs.md), detailing how the plugin works, and what other utilities are used under the hood to provide you with the visualizations and the log entries.

The [`systemd` journal plugin](https://github.com/netdata/netdata/blob/master/src/collectors/systemd-journal.plugin/README.md) documentation has information about:

- [Key features the plugin provides](https://github.com/netdata/netdata/blob/master/src/collectors/systemd-journal.plugin/README.md#key-features)
- [Journal sources](https://github.com/netdata/netdata/blob/master/src/collectors/systemd-journal.plugin/README.md#journal-sources)
- [Journal fields](https://github.com/netdata/netdata/blob/master/src/collectors/systemd-journal.plugin/README.md#journal-fields)
- [Full-text search](https://github.com/netdata/netdata/blob/master/src/collectors/systemd-journal.plugin/README.md#full-text-search)
- [Query performance](https://github.com/netdata/netdata/blob/master/src/collectors/systemd-journal.plugin/README.md#query-performance)
- [Performance at scale](https://github.com/netdata/netdata/blob/master/src/collectors/systemd-journal.plugin/README.md#performance-at-scale)

We recommend you to read through that document, to better understand how the plugin and the visualizations work.
