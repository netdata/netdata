# Command-line options

Normally you don't need to supply any command line arguments to netdata.

If you do though, they override the configuration equivalent options.

To get a list of all command line parameters supported, run:

```sh
netdata -h
```

The program will print the supported command line parameters.

The command line options of the netdata 1.10.0 version are the following:
```
 SYNOPSIS: netdata [options]

 Options:

  -c filename              Configuration file to load.
                           Default: /etc/netdata/netdata.conf

  -D                       Do not fork. Run in the foreground.
                           Default: run in the background

  -h                       Display this help message.

  -P filename              File to save a pid while running.
                           Default: do not save pid to a file

  -i IP                    The IP address to listen to.
                           Default: all IP addresses IPv4 and IPv6

  -p port                  API/Web port to use.
                           Default: 19999

  -s path                  Prefix for /proc and /sys (for containers).
                           Default: no prefix

  -t seconds               The internal clock of netdata.
                           Default: 1

  -u username              Run as user.
                           Default: netdata

  -v                       Print netdata version and exit.

  -V                       Print netdata version and exit.

  -W options               See Advanced options below.


 Advanced options:

  -W stacksize=N           Set the stacksize (in bytes).

  -W debug_flags=N         Set runtime tracing to debug.log.

  -W unittest              Run internal unittests and exit.

  -W set section option value
                           set netdata.conf option from the command line.

  -W simple-pattern pattern string
                           Check if string matches pattern and exit.


 Signals netdata handles:

  - HUP                    Close and reopen log files.
  - USR1                   Save internal DB to disk.
  - USR2                   Reload health configuration.

```
