<!--
title: go.d.plugin
description: "go.d.plugin is an external plugin for Netdata, responsible for running individual data collectors written in Go."
custom_edit_url: "https://github.com/netdata/netdata/blob/master/src/go/collectors/go.d.plugin/README.md"
sidebar_label: "go.d.plugin"
learn_status: "Published"
learn_topic_type: "Tasks"
learn_rel_path: "Developers/External plugins/go.d.plugin"
sidebar_position: 1
-->

# go.d.plugin

`go.d.plugin` is a [Netdata](https://github.com/netdata/netdata) external plugin. It is an **orchestrator** for data
collection modules written in `go`.

1. It runs as an independent process (`ps fax` shows it).
2. It is started and stopped automatically by Netdata.
3. It communicates with Netdata via a unidirectional pipe (sending data to the Netdata daemon).
4. Supports any number of data collection [modules](https://github.com/netdata/netdata/tree/master/src/go/collectors/go.d.plugin/modules).
5. Allows each [module](https://github.com/netdata/netdata/tree/master/src/go/collectors/go.d.plugin/modules) to have any number of data
   collection jobs.

## Bug reports, feature requests, and questions

Are welcome! We are using [netdata/netdata](https://github.com/netdata/netdata/) repository for bugs, feature requests,
and questions.

- [GitHub Issues](https://github.com/netdata/netdata/issues/new/choose): report bugs or open a new feature request.
- [GitHub Discussions](https://github.com/netdata/netdata/discussions): ask a question or suggest a new idea.

## Install

Go.d.plugin is shipped with Netdata.

### Required Linux capabilities

All capabilities are set automatically during Netdata installation using
the [official installation method](https://github.com/netdata/netdata/blob/master/packaging/installer/README.md#install-on-linux-with-one-line-installer).
No further action required. If you have used a different installation method and need to set the capabilities manually,
see the appropriate collector readme.

| Capability          |                                                    Required by                                                     |
|:--------------------|:------------------------------------------------------------------------------------------------------------------:|
| CAP_NET_RAW         |      [Ping](https://github.com/netdata/netdata/tree/master/src/go/collectors/go.d.plugin/modules/ping#readme)      |
| CAP_NET_ADMIN       | [Wireguard](https://github.com/netdata/netdata/tree/master/src/go/collectors/go.d.plugin/modules/wireguard#readme) |
| CAP_DAC_READ_SEARCH | [Filecheck](https://github.com/netdata/netdata/tree/master/src/go/collectors/go.d.plugin/modules/filecheck#readme) |

## Available modules

| Name                                                                                                                          |           Monitors            |
|:------------------------------------------------------------------------------------------------------------------------------|:-----------------------------:|
| [adaptec_raid](https://github.com/netdata/netdata/tree/master/src/go/collectors/go.d.plugin/modules/adaptecraid)              |     Adaptec Hardware RAID     |
| [activemq](https://github.com/netdata/netdata/tree/master/src/go/collectors/go.d.plugin/modules/activemq)                     |           ActiveMQ            |
| [apache](https://github.com/netdata/netdata/tree/master/src/go/collectors/go.d.plugin/modules/apache)                         |            Apache             |
| [bind](https://github.com/netdata/netdata/tree/master/src/go/collectors/go.d.plugin/modules/bind)                             |           ISC Bind            |
| [cassandra](https://github.com/netdata/netdata/tree/master/src/go/collectors/go.d.plugin/modules/cassandra)                   |           Cassandra           |
| [chrony](https://github.com/netdata/netdata/tree/master/src/go/collectors/go.d.plugin/modules/chrony)                         |            Chrony             |
| [cockroachdb](https://github.com/netdata/netdata/tree/master/src/go/collectors/go.d.plugin/modules/cockroachdb)               |          CockroachDB          |
| [consul](https://github.com/netdata/netdata/tree/master/src/go/collectors/go.d.plugin/modules/consul)                         |            Consul             |
| [coredns](https://github.com/netdata/netdata/tree/master/src/go/collectors/go.d.plugin/modules/coredns)                       |            CoreDNS            |
| [couchbase](https://github.com/netdata/netdata/tree/master/src/go/collectors/go.d.plugin/modules/couchbase)                   |           Couchbase           |
| [couchdb](https://github.com/netdata/netdata/tree/master/src/go/collectors/go.d.plugin/modules/couchdb)                       |            CouchDB            |
| [dnsdist](https://github.com/netdata/netdata/tree/master/src/go/collectors/go.d.plugin/modules/dnsdist)                       |            Dnsdist            |
| [dnsmasq](https://github.com/netdata/netdata/tree/master/src/go/collectors/go.d.plugin/modules/dnsmasq)                       |     Dnsmasq DNS Forwarder     |
| [dnsmasq_dhcp](https://github.com/netdata/netdata/tree/master/src/go/collectors/go.d.plugin/modules/dnsmasq_dhcp)             |         Dnsmasq DHCP          |
| [dns_query](https://github.com/netdata/netdata/tree/master/src/go/collectors/go.d.plugin/modules/dnsquery)                    |         DNS Query RTT         |
| [docker](https://github.com/netdata/netdata/tree/master/src/go/collectors/go.d.plugin/modules/docker)                         |         Docker Engine         |
| [docker_engine](https://github.com/netdata/netdata/tree/master/src/go/collectors/go.d.plugin/modules/docker_engine)           |         Docker Engine         |
| [dockerhub](https://github.com/netdata/netdata/tree/master/src/go/collectors/go.d.plugin/modules/dockerhub)                   |          Docker Hub           |
| [elasticsearch](https://github.com/netdata/netdata/tree/master/src/go/collectors/go.d.plugin/modules/elasticsearch)           |   Elasticsearch/OpenSearch    |
| [energid](https://github.com/netdata/netdata/tree/master/src/go/collectors/go.d.plugin/modules/energid)                       |          Energi Core          |
| [envoy](https://github.com/netdata/netdata/tree/master/src/go/collectors/go.d.plugin/modules/envoy)                           |             Envoy             |
| [example](https://github.com/netdata/netdata/tree/master/src/go/collectors/go.d.plugin/modules/example)                       |               -               |
| [fail2ban](https://github.com/netdata/netdata/tree/master/src/go/collectors/go.d.plugin/modules/fail2ban)                     |        Fail2Ban Jails         |
| [filecheck](https://github.com/netdata/netdata/tree/master/src/go/collectors/go.d.plugin/modules/filecheck)                   |     Files and Directories     |
| [fluentd](https://github.com/netdata/netdata/tree/master/src/go/collectors/go.d.plugin/modules/fluentd)                       |            Fluentd            |
| [freeradius](https://github.com/netdata/netdata/tree/master/src/go/collectors/go.d.plugin/modules/freeradius)                 |          FreeRADIUS           |
| [haproxy](https://github.com/netdata/netdata/tree/master/src/go/collectors/go.d.plugin/modules/haproxy)                       |            HAProxy            |
| [hddtemp](https://github.com/netdata/netdata/tree/master/src/go/collectors/go.d.plugin/modules/hddtemp)                       |       Disks temperature       |
| [hdfs](https://github.com/netdata/netdata/tree/master/src/go/collectors/go.d.plugin/modules/hdfs)                             |             HDFS              |
| [httpcheck](https://github.com/netdata/netdata/tree/master/src/go/collectors/go.d.plugin/modules/httpcheck)                   |       Any HTTP Endpoint       |
| [intelgpu](https://github.com/netdata/netdata/tree/master/src/go/collectors/go.d.plugin/modules/intelgpu)                     |     Intel integrated GPU      |
| [isc_dhcpd](https://github.com/netdata/netdata/tree/master/src/go/collectors/go.d.plugin/modules/isc_dhcpd)                   |           ISC DHCP            |
| [k8s_kubelet](https://github.com/netdata/netdata/tree/master/src/go/collectors/go.d.plugin/modules/k8s_kubelet)               |            Kubelet            |
| [k8s_kubeproxy](https://github.com/netdata/netdata/tree/master/src/go/collectors/go.d.plugin/modules/k8s_kubeproxy)           |          Kube-proxy           |
| [k8s_state](https://github.com/netdata/netdata/tree/master/src/go/collectors/go.d.plugin/modules/k8s_state)                   |   Kubernetes cluster state    |
| [lighttpd](https://github.com/netdata/netdata/tree/master/src/go/collectors/go.d.plugin/modules/lighttpd)                     |           Lighttpd            |
| [logind](https://github.com/netdata/netdata/tree/master/src/go/collectors/go.d.plugin/modules/logind)                         |        systemd-logind         |
| [logstash](https://github.com/netdata/netdata/tree/master/src/go/collectors/go.d.plugin/modules/logstash)                     |           Logstash            |
| [lvm](https://github.com/netdata/netdata/tree/master/src/go/collectors/go.d.plugin/modules/lvm)                               |      LVM logical volumes      |
| [megacli](https://github.com/netdata/netdata/tree/master/src/go/collectors/go.d.plugin/modules/megacli)                       |     MegaCli Hardware Raid     |
| [mongoDB](https://github.com/netdata/netdata/tree/master/src/go/collectors/go.d.plugin/modules/mongodb)                       |            MongoDB            |
| [mysql](https://github.com/netdata/netdata/tree/master/src/go/collectors/go.d.plugin/modules/mysql)                           |             MySQL             |
| [nginx](https://github.com/netdata/netdata/tree/master/src/go/collectors/go.d.plugin/modules/nginx)                           |             NGINX             |
| [nginxplus](https://github.com/netdata/netdata/tree/master/src/go/collectors/go.d.plugin/modules/nginxplus)                   |          NGINX Plus           |
| [nginxvts](https://github.com/netdata/netdata/tree/master/src/go/collectors/go.d.plugin/modules/nginxvts)                     |           NGINX VTS           |
| [ntpd](https://github.com/netdata/netdata/tree/master/src/go/collectors/go.d.plugin/modules/ntpd)                             |          NTP daemon           |
| [nvme](https://github.com/netdata/netdata/tree/master/src/go/collectors/go.d.plugin/modules/nvme)                             |         NVMe devices          |
| [openvpn](https://github.com/netdata/netdata/tree/master/src/go/collectors/go.d.plugin/modules/openvpn)                       |            OpenVPN            |
| [openvpn_status_log](https://github.com/netdata/netdata/tree/master/src/go/collectors/go.d.plugin/modules/openvpn_status_log) |            OpenVPN            |
| [pgbouncer](https://github.com/netdata/netdata/tree/master/src/go/collectors/go.d.plugin/modules/pgbouncer)                   |           PgBouncer           |
| [phpdaemon](https://github.com/netdata/netdata/tree/master/src/go/collectors/go.d.plugin/modules/phpdaemon)                   |           phpDaemon           |
| [phpfpm](https://github.com/netdata/netdata/tree/master/src/go/collectors/go.d.plugin/modules/phpfpm)                         |            PHP-FPM            |
| [pihole](https://github.com/netdata/netdata/tree/master/src/go/collectors/go.d.plugin/modules/pihole)                         |            Pi-hole            |
| [pika](https://github.com/netdata/netdata/tree/master/src/go/collectors/go.d.plugin/modules/pika)                             |             Pika              |
| [ping](https://github.com/netdata/netdata/tree/master/src/go/collectors/go.d.plugin/modules/ping)                             |       Any network host        |
| [prometheus](https://github.com/netdata/netdata/tree/master/src/go/collectors/go.d.plugin/modules/prometheus)                 |    Any Prometheus Endpoint    |
| [portcheck](https://github.com/netdata/netdata/tree/master/src/go/collectors/go.d.plugin/modules/portcheck)                   |       Any TCP Endpoint        |
| [postgres](https://github.com/netdata/netdata/tree/master/src/go/collectors/go.d.plugin/modules/postgres)                     |          PostgreSQL           |
| [powerdns](https://github.com/netdata/netdata/tree/master/src/go/collectors/go.d.plugin/modules/powerdns)                     | PowerDNS Authoritative Server |
| [powerdns_recursor](https://github.com/netdata/netdata/tree/master/src/go/collectors/go.d.plugin/modules/powerdns_recursor)   |       PowerDNS Recursor       |
| [proxysql](https://github.com/netdata/netdata/tree/master/src/go/collectors/go.d.plugin/modules/proxysql)                     |           ProxySQL            |
| [pulsar](https://github.com/netdata/netdata/tree/master/src/go/collectors/go.d.plugin/modules/portcheck)                      |         Apache Pulsar         |
| [rabbitmq](https://github.com/netdata/netdata/tree/master/src/go/collectors/go.d.plugin/modules/rabbitmq)                     |           RabbitMQ            |
| [redis](https://github.com/netdata/netdata/tree/master/src/go/collectors/go.d.plugin/modules/redis)                           |             Redis             |
| [scaleio](https://github.com/netdata/netdata/tree/master/src/go/collectors/go.d.plugin/modules/scaleio)                       |       Dell EMC ScaleIO        |
| [sensors](https://github.com/netdata/netdata/tree/master/src/go/collectors/go.d.plugin/modules)                               |       Hardware Sensors        |
| [SNMP](https://github.com/netdata/netdata/blob/master/src/go/collectors/go.d.plugin/modules/snmp)                             |             SNMP              |
| [squidlog](https://github.com/netdata/netdata/tree/master/src/go/collectors/go.d.plugin/modules/squidlog)                     |             Squid             |
| [storcli](https://github.com/netdata/netdata/tree/master/src/go/collectors/go.d.plugin/modules/storcli)                       |    Broadcom Hardware RAID     |
| [supervisord](https://github.com/netdata/netdata/tree/master/src/go/collectors/go.d.plugin/modules/supervisord)               |          Supervisor           |
| [systemdunits](https://github.com/netdata/netdata/tree/master/src/go/collectors/go.d.plugin/modules/systemdunits)             |      Systemd unit state       |
| [tengine](https://github.com/netdata/netdata/tree/master/src/go/collectors/go.d.plugin/modules/tengine)                       |            Tengine            |
| [traefik](https://github.com/netdata/netdata/tree/master/src/go/collectors/go.d.plugin/modules/traefik)                       |            Traefik            |
| [upsd](https://github.com/netdata/netdata/tree/master/src/go/collectors/go.d.plugin/modules/upsd)                             |          UPSd (Nut)           |
| [unbound](https://github.com/netdata/netdata/tree/master/src/go/collectors/go.d.plugin/modules/unbound)                       |            Unbound            |
| [vcsa](https://github.com/netdata/netdata/tree/master/src/go/collectors/go.d.plugin/modules/vcsa)                             |   vCenter Server Appliance    |
| [vernemq](https://github.com/netdata/netdata/tree/master/src/go/collectors/go.d.plugin/modules/vernemq)                       |            VerneMQ            |
| [vsphere](https://github.com/netdata/netdata/tree/master/src/go/collectors/go.d.plugin/modules/vsphere)                       |     VMware vCenter Server     |
| [web_log](https://github.com/netdata/netdata/tree/master/src/go/collectors/go.d.plugin/modules/weblog)                        |         Apache/NGINX          |
| [wireguard](https://github.com/netdata/netdata/tree/master/src/go/collectors/go.d.plugin/modules/wireguard)                   |           WireGuard           |
| [whoisquery](https://github.com/netdata/netdata/tree/master/src/go/collectors/go.d.plugin/modules/whoisquery)                 |         Domain Expiry         |
| [windows](https://github.com/netdata/netdata/tree/master/src/go/collectors/go.d.plugin/modules/windows)                       |            Windows            |
| [x509check](https://github.com/netdata/netdata/tree/master/src/go/collectors/go.d.plugin/modules/x509check)                   |     Digital Certificates      |
| [zfspool](https://github.com/netdata/netdata/tree/master/src/go/collectors/go.d.plugin/modules/zfspool)                       |           ZFS Pools           |
| [zookeeper](https://github.com/netdata/netdata/tree/master/src/go/collectors/go.d.plugin/modules/zookeeper)                   |           ZooKeeper           |

## Configuration

Edit the `go.d.conf` configuration file using `edit-config` from the
Netdata [config directory](https://github.com/netdata/netdata/blob/master/docs/netdata-agent/configuration.md#the-netdata-config-directory), which is typically
at `/etc/netdata`.

```bash
cd /etc/netdata # Replace this path with your Netdata config directory
sudo ./edit-config go.d.conf
```

Configurations are written in [YAML](http://yaml.org/).

- [plugin configuration](https://github.com/netdata/netdata/blob/master/src/go/collectors/go.d.plugin/config/go.d.conf)
- [specific module configuration](https://github.com/netdata/netdata/tree/master/src/go/collectors/go.d.plugin/config/go.d)

### Enable a collector

To enable a collector you should edit `go.d.conf` to uncomment the collector in question and change it from `no`
to `yes`.

For example, to enable the `example` plugin you would need to update `go.d.conf` from something like:

```yaml
modules:
#  example: no 
```

to

```yaml
modules:
  example: yes
```

Then [restart netdata](https://github.com/netdata/netdata/blob/master/packaging/installer/README.md#maintaining-a-netdata-agent-installation) for the
change to take effect.

## Contributing

If you want to contribute to this project, we are humbled. Please take a look at
our [contributing guidelines](https://github.com/netdata/.github/blob/main/CONTRIBUTING.md) and don't hesitate to
contact us in our forums.

### How to develop a collector

Read [how to write a Netdata collector in Go](https://github.com/netdata/netdata/blob/master/src/go/collectors/go.d.plugin/docs/how-to-write-a-module.md).

## Troubleshooting

Plugin CLI:

```sh
Usage:
  orchestrator [OPTIONS] [update every]

Application Options:
  -m, --modules=    module name to run (default: all)
  -c, --config-dir= config dir to read
  -w, --watch-path= config path to watch
  -d, --debug       debug mode
  -v, --version     display the version and exit

Help Options:
  -h, --help        Show this help message
```

To debug specific module:

```sh
# become user netdata
sudo su -s /bin/bash netdata

# run plugin in debug mode
./go.d.plugin -d -m <module name>
```

Change `<module name>` to the module name you want to debug. See the [whole list](#available-modules) of available
modules.

## Netdata Community

This repository follows the Netdata Code of Conduct and is part of the Netdata Community.

- [Community Forums](https://community.netdata.cloud)
- [Netdata Code of Conduct](https://github.com/netdata/.github/blob/main/CODE_OF_CONDUCT.md)
