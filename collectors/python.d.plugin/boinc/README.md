# boinc

This module monitors task counts for the Berkely Open Infrastructure
Networking Computing (BOINC) distributed computing client using the same
RPC interface that the BOINC monitoring GUI does.

It provides charts tracking the total number of tasks and active tasks,
as well as ones tracking each of the possible states for tasks.

### configuration

BOINC requires use of a password to access it's RPC interface.  You can
find this password in the `gui_rpc_auth.cfg` file in your BOINC directory.

By default, the module will try to auto-detect the password by looking
in `/var/lib/boinc` for this file (this is the location most Linux
distributions use for a system-wide BOINC installation), so things may
just work without needing configuration for the local system.

You can monitor remote systems as well:

```yaml
remote:
  hostname: some-host
  password: some-password
```

---

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fcollectors%2Fpython.d.plugin%2Fboinc%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)]()
