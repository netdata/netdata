# Remote Windows WMI plugin

`wmi.plugin` monitors Windows hosts without installing anything on them. It runs on a Netdata Agent (the poller) and
reads CPU, memory, network and disk metrics from each configured host over WMI, using DCOM or WinRM HTTPS. Each host
appears in Netdata as its own virtual node.

What it collects, how to prepare the Windows hosts, every option and the known errors are on the Windows (WMI)
integration page, generated from [`collector/windows/metadata.yaml`](collector/windows/metadata.yaml).

## Build

The plugin is experimental. Netdata packages do not include it, and source builds include it only on request: pass
`--enable-plugin-wmi` to `netdata-installer.sh`, or configure CMake with `-DENABLE_PLUGIN_WMI=ON`.

To build a standalone executable from `src/go` (no C compiler needed):

```sh
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o wmi.plugin ./cmd/wmiplugin
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o wmi.plugin.exe ./cmd/wmiplugin
```

A standalone Linux or macOS build reads `tls_ca` through the installed `nd-run` helper. To test WinRM with a private CA
outside an installation, build with `-ldflags "-X github.com/netdata/netdata/go/plugins/pkg/buildinfo.NetdataBinDir=<dir>"`
pointing at a directory that contains `nd-run`.

## Run one job in the foreground

Run the plugin as the Netdata service account, with the Agent's configuration and environment, so secret references
such as `${env:WMI_PASSWORD}` resolve the same way:

```sh
/usr/libexec/netdata/plugins.d/wmi.plugin -d -m windows -j fileserver
```

The debug output shows the job's checks and collections, and the charts it sends for the host's virtual node.
