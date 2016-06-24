[![Build Status](https://travis-ci.org/firehol/netdata.svg?branch=master)](https://travis-ci.org/firehol/netdata)
<a href="https://scan.coverity.com/projects/firehol-netdata"><img alt="Coverity Scan Build Status" src="https://scan.coverity.com/projects/9140/badge.svg"/></a>
[![User Base](https://registry.my-netdata.io/api/v1/badge.svg?chart=netdata.registry_entries&dimensions=persons&label=user%20base&units=null&value_color=blue&precision=0&v41)](https://registry.my-netdata.io/#netdata_registry)
[![Monitored Servers](https://registry.my-netdata.io/api/v1/badge.svg?chart=netdata.registry_entries&dimensions=machines&label=servers%20monitored&units=null&value_color=orange&precision=0&v41)](https://registry.my-netdata.io/#netdata_registry)
[![Sessions Served](https://registry.my-netdata.io/api/v1/badge.svg?chart=netdata.registry_sessions&label=sessions%20served&units=null&value_color=yellowgreen&precision=0&v41)](https://registry.my-netdata.io/#netdata_registry)

[![New Users Today](http://registry.my-netdata.io/api/v1/badge.svg?chart=netdata.registry_entries&dimensions=persons&after=-86400&options=unaligned&group=incremental-sum&label=new%20users%20today&units=null&value_color=blue&precision=0&v41)](https://registry.my-netdata.io/#netdata_registry)
[![New Machines Today](https://registry.my-netdata.io/api/v1/badge.svg?chart=netdata.registry_entries&dimensions=machines&group=incremental-sum&after=-86400&options=unaligned&label=servers%20added%20today&units=null&value_color=orange&precision=0&v41)](https://registry.my-netdata.io/#netdata_registry)
[![Sessions Today](https://registry.my-netdata.io/api/v1/badge.svg?chart=netdata.registry_sessions&after=-86400&group=incremental-sum&options=unaligned&label=sessions%20served%20today&units=null&value_color=yellowgreen&precision=0&v41)](https://registry.my-netdata.io/#netdata_registry)


# netdata

> Junio 24, 2016
>
> [netdata v1.2.0 released!](https://github.com/firehol/netdata/releases)
>
> - 30% faster!
> - **[netdata registry](https://github.com/firehol/netdata/wiki/mynetdata-menu-item)**, the first step towards scaling out performance monitoring!
> - real-time Linux Containers monitoring!
> - dozens of additional new features, optimizations, bug-fixes

---

May 1st, 2016

##### 320.000+ views, 92.000+ visitors, 28.500+ downloads, 11.000+ github stars, 700+ forks, 1 month!

And it still runs with 600+ git downloads... per day!

**[Check what our users say about netdata](https://github.com/firehol/netdata/issues/148)**.

---

**Real-time performance monitoring, done right!**

This is the default dashboard of **netdata**:

 - real-time, per second updates, snappy refreshes!
 - 300+ charts out of the box, 2000+ metrics monitored!
 - zero configuration, zero maintenance, zero dependencies!

Live demo: [http://netdata.firehol.org](http://netdata.firehol.org)

![netdata](https://cloud.githubusercontent.com/assets/2662304/14092712/93b039ea-f551-11e5-822c-beadbf2b2a2e.gif)

---

## Características:

**netdata** es una aplicación de Linux altamente optimizado **para dar resultados en tiempo real y monitorear los sistemas, aplicaciones, y aparatos de SNMP en la red!**!

trata de visualizar la **verdad en este momento,**, en **gran detalle** y para que tengas una idea que está pasando y paso en tu sistema y aplicaciones.

Esto es lo que recibes:

- **1. Una tabla brillosa que usa Bootstrap** fuera de la caja (Opciones en color claro u obscuro).
- **2. Es ultrarrápido** y  **súper eficiente**, La mayoría fue escrito en el lenguaje C (las instalaciones estándar, espera un uso de 2% de un solo ciclo del CPU y solo unos cuando MB de RAM).
- **3. Zero configuración** - Solo lo instalas y detecta todo automático.
- **Zero dependencias** Tiene su propio Web servidor con los documentos estáticos y API
- **No tienes que mantenerlo** solo correrlo y hace lo demás!
- **Tablas que puedes modificar** Las puedes crear solo usando HTML (no necesitas JavaScript)
- **Extensible** Puedes monitorear cualquier cosa que puedes sacar métricas usando el Plugin API (todo puede ser un Netdata Plugin – de BASH a node.js. Puedes monitorear cualquier aplicación, cualquier API)
- **Puedes ponerlo adentro** de cualquier sistema que tenga un Linux Kernel. Puedes poner sus graficas adentro de tus páginas.

---

## ¿Que monitorea?

Esto es lo que monitorea (la mayoría con cambios mínimos a la configuración):

- **1. El uso del CPU, Interrupciones y frecuencias** (Total y de cada Core)

- **2. El uso de RAM, swap y de la memoria del Kernel** (Incluyendo KSM y la memoria Kernel duper)

- **3. El disco duro** (Cada disco: I/O, Operaciones, Reserva, El uso, espacio, ETC)

   ![sda](https://cloud.githubusercontent.com/assets/2662304/14093195/c882bbf4-f554-11e5-8863-1788d643d2c0.gif)

- **Network interfaces** (por cada uno: tráfico, paquetes, errores y caídas, etc.)

   ![dsl0](https://cloud.githubusercontent.com/assets/2662304/14093128/4d566494-f554-11e5-8ee4-5392e0ac51f0.gif)

- **IPv4 networking** (bandwidth, packets, errors, fragments, tcp: connections, packets, errors, handshake, udp: packets, errors, broadcast: bandwidth, packets, multicast: bandwidth, packets)

- **IPv6 networking** (bandwidth, packets, errors, fragments, ECT, udp: packets, errors, udplite: packets, errors, broadcast: bandwidth, multicast: bandwidth, packets, icmp: messages, errors, echos, router, neighbor, MLDv2, group membership, break down by type)

- **netfilter / iptables Linux firewall** (connections, connection tracker events, errors, etc)

- **Linux DDoS protection** (SYNPROXY metrics)

- **Processes** (running, blocked, forks, active, etc)

- **Entropy** (random numbers pool, using in cryptography)

- **NFS file servers**, v2, v3, v4 (I/O, cache, read ahead, RPC calls)

- **Network QoS** (yes, the only tool that visualizes network `tc` classes in realtime)

   ![qos-tc-classes](https://cloud.githubusercontent.com/assets/2662304/14093004/68966020-f553-11e5-98fe-ffee2086fafd.gif)

- **Linux Control Groups** (containers), systemd, lxc, docker, etc

- **Applications**, by grouping the process tree (CPU, memory, disk reads, disk writes, swap, threads, pipes, sockets, etc)

   ![apps](https://cloud.githubusercontent.com/assets/2662304/14093565/67c4002c-f557-11e5-86bd-0154f5135def.gif)

- **Users and User Groups resource usage**, by summarizing the process tree per user and group (CPU, memory, disk reads, disk writes, swap, threads, pipes, sockets, etc)

- **Apache web server** mod-status (v2.2, v2.4)

- **Nginx web server** stub-status

- **mySQL databases** (multiple servers, each showing: bandwidth, queries/s, handlers, locks, issues, tmp operations, connections, binlog metrics, threads, innodb metrics, etc)

- **ISC Bind name server** (multiple servers, each showing: clients, requests, queries, updates, failures and several per view metrics)

- **Postfix email server** message queue (entries, size)

- **Squid proxy server** (clients bandwidth and requests, servers bandwidth and requests)

- **Hardware sensors** (temperature, voltage, fans, power, humidity, etc)

- **NUT UPSes** (load, charge, battery voltage, temperature, utility metrics, output metrics)

- **Tomcat** (accesses, threads, free memory, volume)

- **PHP-FPM** (multiple instances, each reporting connections, requests, performance)

- **SNMP devices** can be monitored too (although you will need to configure these)

And you can extend it, by writing plugins that collect data from any source, using any computer language.

---

## Still not convinced?

Read **[Why netdata?](https://github.com/firehol/netdata/wiki/Why-netdata%3F)**

---

## Installation

Use our **[automatic installer](https://github.com/firehol/netdata/wiki/Installation)** to build and install it on your system

It should run on **any Linux** system. It has been tested on:

- Gentoo
- Arch Linux
- Ubuntu / Debian
- CentOS
- Fedora
- RedHat Enterprise Linux
- SUSE
- Alpine Linux
- PLD Linux

---

## Documentation

Check the **[netdata wiki](https://github.com/firehol/netdata/wiki)**.
