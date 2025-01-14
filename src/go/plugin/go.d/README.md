# go.d.plugin

`go.d.plugin` is a [Netdata](https://github.com/netdata/netdata) external plugin:

- **Independent Operation**: Runs as a separate process from Netdata core, visible in system process lists (`ps fax`).
- **Automated Management**: Integrated with Netdata's lifecycle management, managed automatically by Netdata (start/stop operations).
- **Efficient Communication**: Uses a unidirectional pipe for optimal data transfer to Netdata.
- **Modular Architecture**:
    - Supports an unlimited number of data collection modules.
    - Each module can run multiple collection jobs simultaneously.
    - Easy to extend with new collection modules

### Required Linux capabilities

All capabilities are set automatically during Netdata installation using the [official installation method](/packaging/installer/methods/kickstart.md).

| Capability          |                                                Required by                                                |
|:--------------------|:---------------------------------------------------------------------------------------------------------:|
| CAP_NET_RAW         |      [Ping](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/ping#readme)      |
| CAP_NET_ADMIN       | [Wireguard](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/wireguard#readme) |
| CAP_DAC_READ_SEARCH | [Filecheck](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/filecheck#readme) |

## Available modules

<details>
<summary>Data Collection Modules</summary>

| Name                                                                                                                 |           Monitors            |
|:---------------------------------------------------------------------------------------------------------------------|:-----------------------------:|
| [adaptec_raid](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/adaptecraid)              |     Adaptec Hardware RAID     |
| [activemq](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/activemq)                     |           ActiveMQ            |
| [ap](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/ap)                                 |          Wireless AP          |
| [apache](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/apache)                         |            Apache             |
| [apcupsd](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/apcupsd)                       |           UPS (APC)           |
| [beanstalk](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/beanstalk)                   |           Beanstalk           |
| [bind](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/bind)                             |           ISC Bind            |
| [boinc](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/boinc)                           |             BOINC             |
| [cassandra](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/cassandra)                   |           Cassandra           |
| [ceph](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/ceph)                             |             Ceph              |
| [chrony](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/chrony)                         |            Chrony             |
| [clickhouse](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/clickhouse)                 |          ClickHouse           |
| [cockroachdb](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/cockroachdb)               |          CockroachDB          |
| [consul](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/consul)                         |            Consul             |
| [coredns](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/coredns)                       |            CoreDNS            |
| [couchbase](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/couchbase)                   |           Couchbase           |
| [couchdb](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/couchdb)                       |            CouchDB            |
| [dmcache](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/dmcache)                       |            DMCache            |
| [dnsdist](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/dnsdist)                       |            Dnsdist            |
| [dnsmasq](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/dnsmasq)                       |     Dnsmasq DNS Forwarder     |
| [dnsmasq_dhcp](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/dnsmasq_dhcp)             |         Dnsmasq DHCP          |
| [dns_query](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/dnsquery)                    |         DNS Query RTT         |
| [docker](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/docker)                         |         Docker Engine         |
| [docker_engine](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/docker_engine)           |         Docker Engine         |
| [dockerhub](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/dockerhub)                   |          Docker Hub           |
| [dovecot](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/dovecot)                       |            Dovecot            |
| [elasticsearch](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/elasticsearch)           |   Elasticsearch/OpenSearch    |
| [envoy](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/envoy)                           |             Envoy             |
| [exim](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/exim)                             |             Exim              |
| [fail2ban](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/fail2ban)                     |        Fail2Ban Jails         |
| [filecheck](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/filecheck)                   |     Files and Directories     |
| [fluentd](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/fluentd)                       |            Fluentd            |
| [freeradius](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/freeradius)                 |          FreeRADIUS           |
| [gearman](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/gearman)                       |            Gearman            |
| [haproxy](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/haproxy)                       |            HAProxy            |
| [hddtemp](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/hddtemp)                       |       Disks temperature       |
| [hdfs](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/hdfs)                             |             HDFS              |
| [hpssa](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/hpssa)                           |        HPE Smart Array        |
| [httpcheck](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/httpcheck)                   |       Any HTTP Endpoint       |
| [icecast](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/icecast)                       |            Icecast            |
| [intelgpu](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/intelgpu)                     |     Intel integrated GPU      |
| [ipfs](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/ipfs)                             |             IPFS              |
| [isc_dhcpd](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/isc_dhcpd)                   |           ISC DHCP            |
| [k8s_kubelet](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/k8s_kubelet)               |            Kubelet            |
| [k8s_kubeproxy](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/k8s_kubeproxy)           |          Kube-proxy           |
| [k8s_state](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/k8s_state)                   |   Kubernetes cluster state    |
| [lighttpd](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/lighttpd)                     |           Lighttpd            |
| [litespeed](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/litespeed)                   |           Litespeed           |
| [logind](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/logind)                         |        systemd-logind         |
| [logstash](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/logstash)                     |           Logstash            |
| [lvm](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/lvm)                               |      LVM logical volumes      |
| [maxscale](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/maxscale)                     |           MaxScale            |
| [megacli](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/megacli)                       |     MegaCli Hardware Raid     |
| [memcached](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/memcached)                   |           Memcached           |
| [mongoDB](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/mongodb)                       |            MongoDB            |
| [monit](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/monit)                           |             Monit             |
| [mysql](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/mysql)                           |             MySQL             |
| [nats](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/nats)                             |             NATS              |
| [nginx](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/nginx)                           |             NGINX             |
| [nginxplus](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/nginxplus)                   |          NGINX Plus           |
| [nginxunit](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/nginxunit)                   |          NGINX Unit           |
| [nginxvts](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/nginxvts)                     |           NGINX VTS           |
| [nsd](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/nsd)                               |       NSD (NLnet Labs)        |
| [ntpd](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/ntpd)                             |          NTP daemon           |
| [nvidia_smi](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/nvidia_smi)                 |          Nvidia SMI           |
| [nvme](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/nvme)                             |         NVMe devices          |
| [openldap](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/openldap)                     |           OpenLDAP            |
| [openvpn](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/openvpn)                       |            OpenVPN            |
| [openvpn_status_log](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/openvpn_status_log) |            OpenVPN            |
| [pgbouncer](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/pgbouncer)                   |           PgBouncer           |
| [oracledb](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/oracledb)                     |           Oracle DB           |
| [phpdaemon](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/phpdaemon)                   |           phpDaemon           |
| [phpfpm](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/phpfpm)                         |            PHP-FPM            |
| [pihole](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/pihole)                         |            Pi-hole            |
| [pika](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/pika)                             |             Pika              |
| [ping](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/ping)                             |       Any network host        |
| [prometheus](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/prometheus)                 |    Any Prometheus Endpoint    |
| [portcheck](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/portcheck)                   |       Any TCP Endpoint        |
| [postgres](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/postgres)                     |          PostgreSQL           |
| [postfix](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/postfix)                       |            Postfix            |
| [powerdns](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/powerdns)                     | PowerDNS Authoritative Server |
| [powerdns_recursor](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/powerdns_recursor)   |       PowerDNS Recursor       |
| [proxysql](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/proxysql)                     |           ProxySQL            |
| [pulsar](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/pulsar)                         |         Apache Pulsar         |
| [puppet](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/puppet)                         |            Puppet             |
| [rabbitmq](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/rabbitmq)                     |           RabbitMQ            |
| [redis](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/redis)                           |             Redis             |
| [rethinkdb](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/rethinkdb)                   |           RethinkDB           |
| [riakkv](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/riakkv)                         |            Riak KV            |
| [rspamd](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/rspamd)                         |            Rspamd             |
| [samba](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/samba)                           |             Samba             |
| [scaleio](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/scaleio)                       |       Dell EMC ScaleIO        |
| [sensors](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/sensors)                       |       Hardware Sensors        |
| [SNMP](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/snmp)                             |             SNMP              |
| [squid](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/squid)                           |             Squid             |
| [squidlog](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/squidlog)                     |             Squid             |
| [smartctl](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/smartctl)                     |   S.M.A.R.T Storage Devices   |
| [spigotmc](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/spigotmc)                     |           SpigotMC            |
| [storcli](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/storcli)                       |    Broadcom Hardware RAID     |
| [supervisord](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/supervisord)               |          Supervisor           |
| [systemdunits](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/systemdunits)             |      Systemd unit state       |
| [tengine](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/tengine)                       |            Tengine            |
| [tomcat](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/tomcat)                         |            Tomcat             |
| [tor](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/tor)                               |              Tor              |
| [traefik](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/traefik)                       |            Traefik            |
| [typesense](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/typesense)                   |           Typesense           |
| [unbound](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/unbound)                       |            Unbound            |
| [upsd](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/upsd)                             |          UPSd (Nut)           |
| [uwsgi](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/uwsgi)                           |             uWSGI             |
| [varnish](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/varnish)                       |            Varnish            |
| [vcsa](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/vcsa)                             |   vCenter Server Appliance    |
| [vernemq](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/vernemq)                       |            VerneMQ            |
| [vsphere](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/vsphere)                       |     VMware vCenter Server     |
| [w1sensor](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/w1sensor)                     |        1-Wire Sensors         |
| [web_log](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/weblog)                        |         Apache/NGINX          |
| [wireguard](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/wireguard)                   |           WireGuard           |
| [whoisquery](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/whoisquery)                 |         Domain Expiry         |
| [x509check](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/x509check)                   |     Digital Certificates      |
| [yugabytedb](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/yugabytedb)                 |          YugabyteDB           |
| [zfspool](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/zfspool)                       |           ZFS Pools           |
| [zookeeper](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/zookeeper)                   |           ZooKeeper           |

</details>

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

Then [restart netdata](/docs/netdata-agent/start-stop-restart.md) for the change to take effect.

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
