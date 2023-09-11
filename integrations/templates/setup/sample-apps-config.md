A custom format is used.

Each configuration line has a form like:

```
group_name: app1 app2 app3
```

Where `group_name` defines an application group, and `app1`, `app2`, and `app3` are process names to match for
that application group.

Each group can be given multiple times, to add more processes to it.

The process names are the ones returned by:

 -  `ps -e` or `/proc/PID/stat`
 -  in case of substring mode (see below): `/proc/PID/cmdline`

To add process names with spaces, enclose them in quotes (single or double):
`'Plex Media Serv' "my other process"`

Note that spaces are not supported for process groups. Use a dash "-" instead.

You can add an asterisk (\*) at the beginning and/or the end of a process to do wildcard matching:

 - `*name` suffix mode: will search for processes ending with 'name' (/proc/PID/stat)
 - `name*` prefix mode: will search for processes beginning with 'name' (/proc/PID/stat)
 - `*name*` substring mode: will search for 'name' in the whole command line (/proc/PID/cmdline)

If you enter even just one `*name*` (substring), apps.plugin will process /proc/PID/cmdline for all processes,
just once (when they are first seen).

To add processes with single quotes, enclose them in double quotes: "process with this ' single quote"

To add processes with double quotes, enclose them in single quotes: 'process with this " double quote'

The order of the entries in this list is important, the first that matches a process is used, so put important
ones at the top. Processes not matched by any row, will inherit it from their parents or children.

The order also controls the order of the dimensions on the generated charts (although applications started after
apps.plugin is started, will be appended to the existing list of dimensions the netdata daemon maintains).
