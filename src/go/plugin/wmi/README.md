# Remote Windows WMI plugin

Run `wmi.plugin` on a Netdata polling host to monitor remote Windows machines through WMI over DCOM. Each job targets one machine and collects CPU, physical memory, traffic per network interface, and space per fixed logical drive. The plugin reads built-in Windows providers. Install it only on the poller.

## Build from source

With a Netdata source checkout, pass `--enable-plugin-wmi` to `netdata-installer.sh`, or configure CMake with `-DENABLE_PLUGIN_WMI=ON`. The source build defaults to disabled. Nightly packages omit this development plugin.

To build a standalone executable from `src/go`:

```sh
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o wmi.plugin ./cmd/wmiplugin
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o wmi.plugin.exe ./cmd/wmiplugin
```

## Configure a target

Create `wmi/windows.conf` under Netdata's user configuration directory. Supply the password through a Netdata secret reference. For this example, expose `WMI_PASSWORD` to the polling Agent's service environment.

```yaml
jobs:
  - name: fileserver
    host: fileserver.example.org
    auth: ntlm
    username: 'EXAMPLE\monitor'
    password: '${env:WMI_PASSWORD}'
    collectors:
      disk: false
```

Omit `collectors` to collect all four groups. Set any of `cpu`, `memory`, `network`, and `disk` to `false` to skip that query. Leave at least one group enabled. The example above collects CPU, memory, and network traffic. Add another job for each additional endpoint.

The job creates a Netdata virtual node identified by the configured target. Use a consistent hostname across restarts. If the address may change, assign an existing named `vnode` before the first collection so its history follows that node.

## Target requirements

- Allow the polling host to reach the Windows RPC endpoint mapper on TCP 135 and the WMI service on its assigned RPC TCP port. The Windows WMI firewall rules can be scoped to the poller.
- Grant the account remote DCOM access and permission to read the required providers in `root\cimv2`, including **Remote Enable**. Local accounts may also be subject to remote UAC restrictions.
- The development build requires explicit `auth: ntlm` and uses NTLMv2. The target and domain policy must permit NTLMv2. Collection uses read-only WQL over DCOM and requires no WinRM HTTPS listener.

See [Microsoft's remote WMI connection guide](https://learn.microsoft.com/en-us/windows/win32/wmisdk/connecting-to-wmi-on-a-remote-computer) and [remote WMI security settings](https://learn.microsoft.com/en-us/windows/win32/wmisdk/securing-a-remote-wmi-connection).

## Check collection

On the poller, run the installed `wmi.plugin -d -m windows -j fileserver` as the Netdata service account, with the same configuration and secrets. The emitted charts should belong to the target's virtual node. In a test session on the target, compare those charts with Task Manager's aggregate CPU and memory, the Network Interface receive/send byte counters, and used/free bytes for fixed local drives. Formatted rate counters reflect the Windows provider's sampling interval. A fixed volume mounted only at a directory has no logical drive letter and is outside the disk chart's scope.
