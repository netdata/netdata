# Netdata Agent CLI

The `netdatacli` executable offers a straightforward way to manage the Netdata Agent's operations.

It is located in the same directory as the `netdata` binary.

Available commands:

| Command                                                                | Description                                                                                                                                                                      |
|------------------------------------------------------------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `help`                                                                 | Display usage information and exit.                                                                                                                                              |
| `reload-health`                                                        | Reloads the Netdata health configuration, updating alerts based on changes made to configuration files.                                                                          |
| `reload-labels`                                                        | Reloads [host labels](/docs/netdata-agent/configuration/organize-systems-metrics-and-alerts.md#custom-labels) from netdata.conf.                                                 |
| `reopen-logs`                                                          | Close and reopen log files.                                                                                                                                                      |
| `shutdown-agent`                                                       | Gracefully shut down the Netdata Agent.                                                                                                                                          |
| `fatal-agent`                                                          | Log the current state and forcefully halt the Netdata Agent.                                                                                                                     |
| `reload-claiming-state`                                                | Reload the Agent's claiming state from disk.                                                                                                                                     |
| `ping`                                                                 | Checks the Agent's status. If the Agent is alive, it exits with status code 0 and prints 'pong' to standard output. Exits with status code 255 otherwise.                        |
| `aclk-state [json]`                                                    | Return the current state of ACLK and Cloud connection. Optionally in JSON.                                                                                                       |
| `dumpconfig`                                                           | Display the current netdata.conf configuration.                                                                                                                                  |
| `remove-stale-node <node_id \| machine_guid \| hostname \| ALL_NODES>` | Un-registers a stale child Node, removing it from the parent Node's UI and Netdata Cloud. This is useful for ephemeral Nodes that may stop streaming and remain visible as stale. |
| `version`                                                              | Display the Netdata Agent version.                                                                                                                                               |

See also the Netdata daemon [command line options](/src/daemon/README.md#command-line-options).
