<!--
title: go.d.plugin
description: "go.d.plugin is an external plugin for Netdata, responsible for running individual data collectors written in Go."
custom_edit_url: https://github.com/netdata/go.d.plugin/edit/master/README.md
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
4. Supports any number of data collection [modules](https://github.com/netdata/go.d.plugin/tree/master/modules).
5. Allows each [module](https://github.com/netdata/go.d.plugin/tree/master/modules) to have any number of data
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

| Capability          |                                       Required by                                        |
|:--------------------|:----------------------------------------------------------------------------------------:|
| CAP_NET_RAW         |      [Ping](https://github.com/netdata/go.d.plugin/tree/master/modules/ping#readme)      |
| CAP_NET_ADMIN       | [Wireguard](https://github.com/netdata/go.d.plugin/tree/master/modules/wireguard#readme) |
| CAP_DAC_READ_SEARCH | [Filecheck](https://github.com/netdata/go.d.plugin/tree/master/modules/filecheck#readme) |

## Available modules

| Name                                                                                                |           Monitors            |
|:----------------------------------------------------------------------------------------------------|:-----------------------------:|
| [activemq](https://github.com/netdata/go.d.plugin/tree/master/modules/activemq)                     |           ActiveMQ            |
| [apache](https://github.com/netdata/go.d.plugin/tree/master/modules/apache)                         |            Apache             |
| [bind](https://github.com/netdata/go.d.plugin/tree/master/modules/bind)                             |           ISC Bind            |
| [cassandra](https://github.com/netdata/go.d.plugin/tree/master/modules/cassandra)                   |           Cassandra           |
| [chrony](https://github.com/netdata/go.d.plugin/tree/master/modules/chrony)                         |            Chrony             |
| [cockroachdb](https://github.com/netdata/go.d.plugin/tree/master/modules/cockroachdb)               |          CockroachDB          |
| [consul](https://github.com/netdata/go.d.plugin/tree/master/modules/consul)                         |            Consul             |
| [coredns](https://github.com/netdata/go.d.plugin/tree/master/modules/coredns)                       |            CoreDNS            |
| [couchbase](https://github.com/netdata/go.d.plugin/tree/master/modules/couchbase)                   |           Couchbase           |
| [couchdb](https://github.com/netdata/go.d.plugin/tree/master/modules/couchdb)                       |            CouchDB            |
| [dnsdist](https://github.com/netdata/go.d.plugin/tree/master/modules/dnsdist)                       |            Dnsdist            |
| [dnsmasq](https://github.com/netdata/go.d.plugin/tree/master/modules/dnsmasq)                       |     Dnsmasq DNS Forwarder     |
| [dnsmasq_dhcp](https://github.com/netdata/go.d.plugin/tree/master/modules/dnsmasq_dhcp)             |         Dnsmasq DHCP          |
| [dns_query](https://github.com/netdata/go.d.plugin/tree/master/modules/dnsquery)                    |         DNS Query RTT         |
| [docker](https://github.com/netdata/go.d.plugin/tree/master/modules/docker)                         |         Docker Engine         |
| [docker_engine](https://github.com/netdata/go.d.plugin/tree/master/modules/docker_engine)           |         Docker Engine         |
| [dockerhub](https://github.com/netdata/go.d.plugin/tree/master/modules/dockerhub)                   |          Docker Hub           |
| [elasticsearch](https://github.com/netdata/go.d.plugin/tree/master/modules/elasticsearch)           |   Elasticsearch/OpenSearch    |
| [energid](https://github.com/netdata/go.d.plugin/tree/master/modules/energid)                       |          Energi Core          |
| [envoy](https://github.com/netdata/go.d.plugin/tree/master/modules/envoy)                           |             Envoy             |
| [example](https://github.com/netdata/go.d.plugin/tree/master/modules/example)                       |               -               |
| [filecheck](https://github.com/netdata/go.d.plugin/tree/master/modules/filecheck)                   |     Files and Directories     |
| [fluentd](https://github.com/netdata/go.d.plugin/tree/master/modules/fluentd)                       |            Fluentd            |
| [freeradius](https://github.com/netdata/go.d.plugin/tree/master/modules/freeradius)                 |          FreeRADIUS           |
| [haproxy](https://github.com/netdata/go.d.plugin/tree/master/modules/haproxy)                       |            HAProxy            |
| [hdfs](https://github.com/netdata/go.d.plugin/tree/master/modules/hdfs)                             |             HDFS              |
| [httpcheck](https://github.com/netdata/go.d.plugin/tree/master/modules/httpcheck)                   |       Any HTTP Endpoint       |
| [isc_dhcpd](https://github.com/netdata/go.d.plugin/tree/master/modules/isc_dhcpd)                   |           ISC DHCP            |
| [k8s_kubelet](https://github.com/netdata/go.d.plugin/tree/master/modules/k8s_kubelet)               |            Kubelet            |
| [k8s_kubeproxy](https://github.com/netdata/go.d.plugin/tree/master/modules/k8s_kubeproxy)           |          Kube-proxy           |
| [k8s_state](https://github.com/netdata/go.d.plugin/tree/master/modules/k8s_state)                   |   Kubernetes cluster state    |
| [lighttpd](https://github.com/netdata/go.d.plugin/tree/master/modules/lighttpd)                     |           Lighttpd            |
| [logind](https://github.com/netdata/go.d.plugin/tree/master/modules/logind)                         |        systemd-logind         |
| [logstash](https://github.com/netdata/go.d.plugin/tree/master/modules/logstash)                     |           Logstash            |
| [mongoDB](https://github.com/netdata/go.d.plugin/tree/master/modules/mongodb)                       |            MongoDB            |
| [mysql](https://github.com/netdata/go.d.plugin/tree/master/modules/mysql)                           |             MySQL             |
| [nginx](https://github.com/netdata/go.d.plugin/tree/master/modules/nginx)                           |             NGINX             |
| [nginxplus](https://github.com/netdata/go.d.plugin/tree/master/modules/nginxplus)                   |          NGINX Plus           |
| [nginxvts](https://github.com/netdata/go.d.plugin/tree/master/modules/nginxvts)                     |           NGINX VTS           |
| [ntpd](https://github.com/netdata/go.d.plugin/tree/master/modules/ntpd)                             |          NTP daemon           |
| [nvme](https://github.com/netdata/go.d.plugin/tree/master/modules/nvme)                             |         NVMe devices          |
| [openvpn](https://github.com/netdata/go.d.plugin/tree/master/modules/openvpn)                       |            OpenVPN            |
| [openvpn_status_log](https://github.com/netdata/go.d.plugin/tree/master/modules/openvpn_status_log) |            OpenVPN            |
| [pgbouncer](https://github.com/netdata/go.d.plugin/tree/master/modules/pgbouncer)                   |           PgBouncer           |
| [phpdaemon](https://github.com/netdata/go.d.plugin/tree/master/modules/phpdaemon)                   |           phpDaemon           |
| [phpfpm](https://github.com/netdata/go.d.plugin/tree/master/modules/phpfpm)                         |            PHP-FPM            |
| [pihole](https://github.com/netdata/go.d.plugin/tree/master/modules/pihole)                         |            Pi-hole            |
| [pika](https://github.com/netdata/go.d.plugin/tree/master/modules/pika)                             |             Pika              |
| [ping](https://github.com/netdata/go.d.plugin/tree/master/modules/ping)                             |       Any network host        |
| [prometheus](https://github.com/netdata/go.d.plugin/tree/master/modules/prometheus)                 |    Any Prometheus Endpoint    |
| [portcheck](https://github.com/netdata/go.d.plugin/tree/master/modules/portcheck)                   |       Any TCP Endpoint        |
| [postgres](https://github.com/netdata/go.d.plugin/tree/master/modules/postgres)                     |          PostgreSQL           |
| [powerdns](https://github.com/netdata/go.d.plugin/tree/master/modules/powerdns)                     | PowerDNS Authoritative Server |
| [powerdns_recursor](https://github.com/netdata/go.d.plugin/tree/master/modules/powerdns_recursor)   |       PowerDNS Recursor       |
| [proxysql](https://github.com/netdata/go.d.plugin/tree/master/modules/proxysql)                     |           ProxySQL            |
| [pulsar](https://github.com/netdata/go.d.plugin/tree/master/modules/portcheck)                      |         Apache Pulsar         |
| [rabbitmq](https://github.com/netdata/go.d.plugin/tree/master/modules/rabbitmq)                     |           RabbitMQ            |
| [redis](https://github.com/netdata/go.d.plugin/tree/master/modules/redis)                           |             Redis             |
| [scaleio](https://github.com/netdata/go.d.plugin/tree/master/modules/scaleio)                       |       Dell EMC ScaleIO        |
| [SNMP](https://github.com/netdata/go.d.plugin/blob/master/modules/snmp)                             |             SNMP              |
| [solr](https://github.com/netdata/go.d.plugin/tree/master/modules/solr)                             |             Solr              |
| [squidlog](https://github.com/netdata/go.d.plugin/tree/master/modules/squidlog)                     |             Squid             |
| [springboot2](https://github.com/netdata/go.d.plugin/tree/master/modules/springboot2)               |         Spring Boot2          |
| [supervisord](https://github.com/netdata/go.d.plugin/tree/master/modules/supervisord)               |          Supervisor           |
| [systemdunits](https://github.com/netdata/go.d.plugin/tree/master/modules/systemdunits)             |      Systemd unit state       |
| [tengine](https://github.com/netdata/go.d.plugin/tree/master/modules/tengine)                       |            Tengine            |
| [traefik](https://github.com/netdata/go.d.plugin/tree/master/modules/traefik)                       |            Traefik            |
| [upsd](https://github.com/netdata/go.d.plugin/tree/master/modules/upsd)                             |          UPSd (Nut)           |
| [unbound](https://github.com/netdata/go.d.plugin/tree/master/modules/unbound)                       |            Unbound            |
| [vcsa](https://github.com/netdata/go.d.plugin/tree/master/modules/vcsa)                             |   vCenter Server Appliance    |
| [vernemq](https://github.com/netdata/go.d.plugin/tree/master/modules/vernemq)                       |            VerneMQ            |
| [vsphere](https://github.com/netdata/go.d.plugin/tree/master/modules/vsphere)                       |     VMware vCenter Server     |
| [web_log](https://github.com/netdata/go.d.plugin/tree/master/modules/weblog)                        |         Apache/NGINX          |
| [wireguard](https://github.com/netdata/go.d.plugin/tree/master/modules/wireguard)                   |           WireGuard           |
| [whoisquery](https://github.com/netdata/go.d.plugin/tree/master/modules/whoisquery)                 |         Domain Expiry         |
| [windows](https://github.com/netdata/go.d.plugin/tree/master/modules/windows)                       |            Windows            |
| [x509check](https://github.com/netdata/go.d.plugin/tree/master/modules/x509check)                   |     Digital Certificates      |
| [zookeeper](https://github.com/netdata/go.d.plugin/tree/master/modules/zookeeper)                   |           ZooKeeper           |

## Configuration

Edit the `go.d.conf` configuration file using `edit-config` from the
Netdata [config directory](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md), which is typically
at `/etc/netdata`.

```bash
cd /etc/netdata # Replace this path with your Netdata config directory
sudo ./edit-config go.d.conf
```

Configurations are written in [YAML](http://yaml.org/).

- [plugin configuration](https://github.com/netdata/go.d.plugin/blob/master/config/go.d.conf)
- [specific module configuration](https://github.com/netdata/go.d.plugin/tree/master/config/go.d)

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

Then [restart netdata](https://github.com/netdata/netdata/blob/master/docs/configure/start-stop-restart.md) for the
change to take effect.

## Contributing

If you want to contribute to this project, we are humbled. Please take a look at
our [contributing guidelines](https://github.com/netdata/.github/blob/main/CONTRIBUTING.md) and don't hesitate to
contact us in our forums.

### How to develop a collector

Read [how to write a Netdata collector in Go](https://github.com/netdata/go.d.plugin/blob/master/docs/how-to-write-a-module.md).

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
