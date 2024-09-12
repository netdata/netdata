<!--
title: go.d.plugin
description: "go.d.plugin is an external plugin for Netdata, responsible for running individual data collectors written in Go."
custom_edit_url: "/src/go/plugin/go.d/README.md"
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
4. Supports any number of data collection modules.
5. Allows each module to have any number of data collection jobs.

## Bug reports, feature requests, and questions

Are welcome! We are using [netdata/netdata](https://github.com/netdata/netdata/) repository for bugs, feature requests,
and questions.

- [GitHub Issues](https://github.com/netdata/netdata/issues/new/choose): report bugs or open a new feature request.
- [GitHub Discussions](https://github.com/netdata/netdata/discussions): ask a question or suggest a new idea.

## Install

Go.d.plugin is shipped with Netdata.

### Required Linux capabilities

All capabilities are set automatically during Netdata installation using
the [official installation method](/packaging/installer/methods/kickstart.md).
No further action required. If you have used a different installation method and need to set the capabilities manually,
see the appropriate collector readme.

| Capability          |                                               Required by                                               |
|:--------------------|:-------------------------------------------------------------------------------------------------------:|
| CAP_NET_RAW         |      [Ping](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/ping#readme)      |
| CAP_NET_ADMIN       | [Wireguard](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/wireguard#readme) |
| CAP_DAC_READ_SEARCH | [Filecheck](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/filecheck#readme) |

## Available modules

| Name                                                                                                               |           Monitors            |
|:-------------------------------------------------------------------------------------------------------------------|:-----------------------------:|
| [adaptec_raid](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/adaptecraid)              |     Adaptec Hardware RAID     |
| [activemq](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/activemq)                     |           ActiveMQ            |
| [ap](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/ap)                                 |          Wireless AP          |
| [apache](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/apache)                         |            Apache             |
| [apcupsd](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/apcupsd)                       |           UPS (APC)           |
| [beanstalk](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/beanstalk)                   |           Beanstalk           |
| [bind](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/bind)                             |           ISC Bind            |
| [boinc](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/boinc)                           |             BOINC             |
| [cassandra](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/cassandra)                   |           Cassandra           |
| [chrony](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/chrony)                         |            Chrony             |
| [clickhouse](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/clickhouse)                 |          ClickHouse           |
| [cockroachdb](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/cockroachdb)               |          CockroachDB          |
| [consul](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/consul)                         |            Consul             |
| [coredns](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/coredns)                       |            CoreDNS            |
| [couchbase](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/couchbase)                   |           Couchbase           |
| [couchdb](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/couchdb)                       |            CouchDB            |
| [dmcache](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/dmcache)                       |            DMCache            |
| [dnsdist](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/dnsdist)                       |            Dnsdist            |
| [dnsmasq](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/dnsmasq)                       |     Dnsmasq DNS Forwarder     |
| [dnsmasq_dhcp](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/dnsmasq_dhcp)             |         Dnsmasq DHCP          |
| [dns_query](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/dnsquery)                    |         DNS Query RTT         |
| [docker](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/docker)                         |         Docker Engine         |
| [docker_engine](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/docker_engine)           |         Docker Engine         |
| [dockerhub](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/dockerhub)                   |          Docker Hub           |
| [dovecot](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/dovecot)                       |            Dovecot            |
| [elasticsearch](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/elasticsearch)           |   Elasticsearch/OpenSearch    |
| [envoy](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/envoy)                           |             Envoy             |
| [example](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/example)                       |               -               |
| [exim](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/exim)                             |             Exim              |
| [fail2ban](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/fail2ban)                     |        Fail2Ban Jails         |
| [filecheck](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/filecheck)                   |     Files and Directories     |
| [fluentd](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/fluentd)                       |            Fluentd            |
| [freeradius](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/freeradius)                 |          FreeRADIUS           |
| [gearman](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/gearman)                       |            Gearman            |
| [haproxy](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/haproxy)                       |            HAProxy            |
| [hddtemp](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/hddtemp)                       |       Disks temperature       |
| [hdfs](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/hdfs)                             |             HDFS              |
| [hpssa](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/hpssa)                           |        HPE Smart Array        |
| [httpcheck](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/httpcheck)                   |       Any HTTP Endpoint       |
| [icecast](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/icecast)                       |            Icecast            |
| [intelgpu](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/intelgpu)                     |     Intel integrated GPU      |
| [ipfs](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/ipfs)                             |             IPFS              |
| [isc_dhcpd](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/isc_dhcpd)                   |           ISC DHCP            |
| [k8s_kubelet](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/k8s_kubelet)               |            Kubelet            |
| [k8s_kubeproxy](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/k8s_kubeproxy)           |          Kube-proxy           |
| [k8s_state](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/k8s_state)                   |   Kubernetes cluster state    |
| [lighttpd](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/lighttpd)                     |           Lighttpd            |
| [litespeed](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/litespeed)                   |           Litespeed           |
| [logind](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/logind)                         |        systemd-logind         |
| [logstash](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/logstash)                     |           Logstash            |
| [lvm](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/lvm)                               |      LVM logical volumes      |
| [megacli](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/megacli)                       |     MegaCli Hardware Raid     |
| [memcached](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/memcached)                   |           Memcached           |
| [mongoDB](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/mongodb)                       |            MongoDB            |
| [monit](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/monit)                           |             Monit             |
| [mysql](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/mysql)                           |             MySQL             |
| [nginx](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/nginx)                           |             NGINX             |
| [nginxplus](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/nginxplus)                   |          NGINX Plus           |
| [nginxvts](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/nginxvts)                     |           NGINX VTS           |
| [nsd](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/nsd)                               |       NSD (NLnet Labs)        |
| [ntpd](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/ntpd)                             |          NTP daemon           |
| [nvme](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/nvme)                             |         NVMe devices          |
| [openvpn](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/openvpn)                       |            OpenVPN            |
| [openvpn_status_log](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/openvpn_status_log) |            OpenVPN            |
| [pgbouncer](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/pgbouncer)                   |           PgBouncer           |
| [phpdaemon](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/phpdaemon)                   |           phpDaemon           |
| [phpfpm](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/phpfpm)                         |            PHP-FPM            |
| [pihole](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/pihole)                         |            Pi-hole            |
| [pika](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/pika)                             |             Pika              |
| [ping](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/ping)                             |       Any network host        |
| [prometheus](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/prometheus)                 |    Any Prometheus Endpoint    |
| [portcheck](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/portcheck)                   |       Any TCP Endpoint        |
| [postgres](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/postgres)                     |          PostgreSQL           |
| [postfix](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/postfix)                       |            Postfix            |
| [powerdns](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/powerdns)                     | PowerDNS Authoritative Server |
| [powerdns_recursor](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/powerdns_recursor)   |       PowerDNS Recursor       |
| [proxysql](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/proxysql)                     |           ProxySQL            |
| [pulsar](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/pulsar)                         |         Apache Pulsar         |
| [puppet](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/puppet)                         |            Puppet             |
| [rabbitmq](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/rabbitmq)                     |           RabbitMQ            |
| [redis](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/redis)                           |             Redis             |
| [rethinkdb](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/rethinkdb)                   |           RethinkDB           |
| [riakkv](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/riakkv)                         |            Riak KV            |
| [rspamd](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/rspamd)                         |            Rspamd             |
| [samba](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/samba)                           |             Samba             |
| [scaleio](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/scaleio)                       |       Dell EMC ScaleIO        |
| [sensors](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/sensors)                       |       Hardware Sensors        |
| [SNMP](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/modules/snmp)                             |             SNMP              |
| [squid](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/squid)                           |             Squid             |
| [squidlog](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/squidlog)                     |             Squid             |
| [smartctl](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/smartctl)                     |   S.M.A.R.T Storage Devices   |
| [storcli](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/storcli)                       |    Broadcom Hardware RAID     |
| [supervisord](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/supervisord)               |          Supervisor           |
| [systemdunits](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/systemdunits)             |      Systemd unit state       |
| [tengine](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/tengine)                       |            Tengine            |
| [tomcat](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/tomcat)                         |            Tomcat             |
| [tor](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/tor)                               |              Tor              |
| [traefik](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/traefik)                       |            Traefik            |
| [typesense](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/typesense)                   |           Typesense           |
| [unbound](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/unbound)                       |            Unbound            |
| [upsd](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/upsd)                             |          UPSd (Nut)           |
| [uwsgi](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/uwsgi)                           |             uWSGI             |
| [varnish](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/varnish)                       |            Varnish            |
| [vcsa](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/vcsa)                             |   vCenter Server Appliance    |
| [vernemq](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/vernemq)                       |            VerneMQ            |
| [vsphere](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/vsphere)                       |     VMware vCenter Server     |
| [w1sensor](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/w1sensor)                     |        1-Wire Sensors         |
| [web_log](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/weblog)                        |         Apache/NGINX          |
| [wireguard](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/wireguard)                   |           WireGuard           |
| [whoisquery](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/whoisquery)                 |         Domain Expiry         |
| [windows](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/windows)                       |            Windows            |
| [x509check](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/x509check)                   |     Digital Certificates      |
| [zfspool](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/zfspool)                       |           ZFS Pools           |
| [zookeeper](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/modules/zookeeper)                   |           ZooKeeper           |

## Configuration

Edit the `go.d.conf` configuration file using `edit-config` from the
Netdata [config directory](/docs/netdata-agent/configuration/README.md#the-netdata-config-directory),
which is typically at `/etc/netdata`.

```bash
cd /etc/netdata # Replace this path with your Netdata config directory
sudo ./edit-config go.d.conf
```

Configurations are written in [YAML](http://yaml.org/).

- [plugin configuration](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/config/go.d.conf)
- [specific module configuration](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/config/go.d)

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

Then [restart netdata](/docs/netdata-agent/start-stop-restart.md)
for the change to take effect.

## Contributing

If you want to contribute to this project, we are humbled. Please take a look at
our [contributing guidelines](https://github.com/netdata/.github/blob/main/CONTRIBUTING.md) and don't hesitate to
contact us in our forums.

### How to develop a collector

Read [how to write a Netdata collector in Go](/src/go/plugin/go.d/docs/how-to-write-a-module.md).

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

Change `<module name>` to the [module name](#available-modules) you want to debug.

## Netdata Community

This repository follows the Netdata Code of Conduct and is part of the Netdata Community.

- [Community Forums](https://community.netdata.cloud)
- [Netdata Code of Conduct](https://github.com/netdata/.github/blob/main/CODE_OF_CONDUCT.md)
