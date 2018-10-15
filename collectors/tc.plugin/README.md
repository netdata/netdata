## tc.plugin

Netdata monitors `tc` QoS classes for all interfaces.

If you also use [FireQOS](http://firehol.org/tutorial/fireqos-new-user/)) it will collect interface and class names.

There is a [shell helper](tc-qos-helper.sh.in) for this (all parsing is done by the plugin in `C` code - this shell script is just a configuration for the command to run to get `tc` output).

The source of the tc plugin is [here](plugin_tc.c). It is somewhat complex, because a state machine was needed to keep track of all the `tc` classes, including the pseudo classes tc dynamically creates.
