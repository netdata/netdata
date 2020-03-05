# Netdata cli

You can see the commands netdatacli supports by executing it with `netdatacli` and entering `help` in
standard input. All commands are given as standard input to `netdatacli`.

The commands that a running netdata agent can execute are the following:

```sh
The commands are (arguments are in brackets):
help
    Show this help menu.
reload-health
    Reload health configuration.
save-database
    Save internal DB to disk for for memory mode save.
reopen-logs
    Close and reopen log files.
shutdown-agent
    Cleanup and exit the netdata agent.
fatal-agent
    Log the state and halt the netdata agent.
reload-claiming-state
    Reload agent claiming state from disk.
```

Those commands are the same that can be sent to netdata via [signals](../daemon/README.md#command-line-options).

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fcli%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
