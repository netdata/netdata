# Configure the Netdata Agent

Netdata's zero-configuration collection, storage, and visualization features work for many users, infrastructures, and
use cases, but there are some situations where you might want to configure the Netdata Agent running on your node(s),
which can be a physical or virtual machine (VM), container, cloud deployment, or edge/IoT device.

For example, you might want to increase metrics retention, configure a collector based on your infrastructure's unique
setup, or secure the local dashboard by restricting it to only connections from `localhost`.

Whatever the reason, Netdata users should know how to configure individual nodes to act decisively if an incident,
anomaly, or change in infrastructure affects how their Agents should perform.

## The Netdata config directory

On most Linux systems, using our [recommended one-line
installation](https://github.com/netdata/netdata/blob/master/packaging/installer/README.md#install-on-linux-with-one-line-installer), the **Netdata config
directory** is `/etc/netdata/`. The config directory contains several configuration files with the `.conf` extension, a
few directories, and a shell script named `edit-config`.

> Some operating systems will use `/opt/netdata/etc/netdata/` as the config directory. If you're not sure where yours
> is, navigate to `http://NODE:19999/netdata.conf` in your browser, replacing `NODE` with the IP address or hostname of
> your node, and find the `# config directory = ` setting. The value listed is the config directory for your system.

All of Netdata's documentation assumes that your config directory is at `/etc/netdata`, and that you're running any
scripts from inside that directory.

## Netdata's configuration files

Upon installation, the Netdata config directory contains a few files and directories. It's okay if you don't see all
these files in your own Netdata config directory, as the next section describes how to edit any that might not already
exist.

- `netdata.conf` is the main configuration file. This is where you'll find most configuration options. Read descriptions
  for each in the [daemon config](https://github.com/netdata/netdata/blob/master/src/daemon/config/README.md) doc.
- `edit-config` is a shell script used for [editing configuration files](#use-edit-config-to-edit-configuration-files).
- Various configuration files ending in `.conf` for [configuring plugins or
  collectors](https://github.com/netdata/netdata/blob/master/collectors/REFERENCE.md) behave. Examples: `go.d.conf`,
  `python.d.conf`, and `ebpf.d.conf`.
- Various directories ending in `.d`, which contain other configuration files, each ending in `.conf`, for [configuring
  specific collectors](https://github.com/netdata/netdata/blob/master/collectors/REFERENCE.md).
- `apps_groups.conf` is a configuration file for changing how applications/processes are grouped when viewing the
  **Application** charts from [`apps.plugin`](https://github.com/netdata/netdata/blob/master/collectors/apps.plugin/README.md) or
  [`ebpf.plugin`](https://github.com/netdata/netdata/blob/master/collectors/ebpf.plugin/README.md).
- `health.d/` is a directory that contains [health configuration files](https://github.com/netdata/netdata/blob/master/health/REFERENCE.md).
- `health_alarm_notify.conf` enables and configures [alert notifications](https://github.com/netdata/netdata/blob/master/docs/monitor/enable-notifications.md).
- `statsd.d/` is a directory for configuring Netdata's [statsd collector](https://github.com/netdata/netdata/blob/master/collectors/statsd.plugin/README.md).
- `stream.conf` configures [parent-child streaming](https://github.com/netdata/netdata/blob/master/src/streaming/README.md) between separate nodes running the Agent.
- `.environment` is a hidden file that describes the environment in which the Netdata Agent is installed, including the
  `PATH` and any installation options. Useful for [reinstalling](https://github.com/netdata/netdata/blob/master/packaging/installer/REINSTALL.md) or
  [uninstalling](https://github.com/netdata/netdata/blob/master/packaging/installer/UNINSTALL.md) the Agent.

The Netdata config directory also contains one symlink:

- `orig` is a symbolic link to the directory `/usr/lib/netdata/conf.d`, which contains stock configuration files. Stock
  versions are copied into the config directory when opened with `edit-config`. _Do not edit the files in
  `/usr/lib/netdata/conf.d`, as they are overwritten by updates to the Netdata Agent._

## Configure a Netdata docker container

See [configure agent containers](https://github.com/netdata/netdata/blob/master/packaging/docker/README.md#configure-agent-containers).

## Use `edit-config` to edit configuration files

The **recommended way to easily and safely edit Netdata's configuration** is with the `edit-config` script. This script
opens existing Netdata configuration files using your system's `$EDITOR`. If the file doesn't yet exist in your config
directory, the script copies the stock version from `/usr/lib/netdata/conf.d` (or wherever the symlink `orig` under the config directory leads to)
to the proper place in the config directory and opens the copy for editing. 

If you have trouble running the script, you can manually copy the file and edit the copy.

e.g. `cp /usr/lib/netdata/conf.d/go.d/bind.conf /etc/netdata/go.d/bind.conf; vi /etc/netdata/go.d/bind.conf`

Run `edit-config` without options, to see details on its usage, or `edit-config --list` to see a list of all the configuration 
files you can edit.

```bash
USAGE:
  ./edit-config [options] FILENAME

  Copy and edit the stock config file named: FILENAME
  if FILENAME is already copied, it will be edited as-is.

  Stock config files at: '/etc/netdata/../../usr/lib/netdata/conf.d'
  User  config files at: '/etc/netdata'

  The editor to use can be specified either by setting the EDITOR
  environment variable, or by using the --editor option.

  The file to edit can also be specified using the --file option.

  For a list of known config files, run './edit-config --list'
```

To edit `netdata.conf`, run `./edit-config netdata.conf`. You may need to elevate your privileges with `sudo` or another
method for `edit-config` to write into the config directory. Use your `$EDITOR`, make your changes, and save the file.

> `edit-config` uses the `EDITOR` environment variable on your system to edit the file. On many systems, that is
> defaulted to `vim` or `nano`. Use `export EDITOR=` to change this temporarily, or edit your shell configuration file
> to change to permanently.

After you make your changes, you need to [restart the Agent](https://github.com/netdata/netdata/blob/master/docs/configure/start-stop-restart.md) with `sudo systemctl
restart netdata` or the appropriate method for your system.

Here's an example of editing the node's hostname, which appears in both the local dashboard and in Netdata Cloud.

![Animated GIF of editing the hostname option in
netdata.conf](https://user-images.githubusercontent.com/1153921/80994808-1c065300-8df2-11ea-81af-d28dc3ba27c8.gif)

### Other configuration files

You can edit any Netdata configuration file using `edit-config`. A few examples:

```bash
./edit-config apps_groups.conf
./edit-config ebpf.d.conf
./edit-config health.d/load.conf
./edit-config go.d/prometheus.conf
```

The documentation for each of Netdata's components explains which file(s) to edit to achieve the desired behavior.

## See an Agent's running configuration

On start, the Netdata Agent daemon attempts to load `netdata.conf`. If that file is missing, incomplete, or contains
invalid settings, the daemon attempts to run sane defaults instead. In other words, the state of `netdata.conf` on your
filesystem may be different from the state of the Netdata Agent itself.

To see the _running configuration_, navigate to `http://NODE:19999/netdata.conf` in your browser, replacing `NODE` with
the IP address or hostname of your node. The file displayed here is exactly the settings running live in the Netdata
Agent.

If you're having issues with configuring the Agent, apply the running configuration to `netdata.conf` by downloading the
file to the Netdata config directory. Use `sudo` to elevate privileges.

```bash
wget -O /etc/netdata/netdata.conf http://localhost:19999/netdata.conf
# or
curl -o /etc/netdata/netdata.conf http://NODE:19999/netdata.conf
```
