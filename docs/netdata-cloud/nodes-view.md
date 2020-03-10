<!--
---
title: "Using the Nodes View"
custom_edit_url: https://github.com/netdata/netdata/edit/master/docs/netdata-cloud/nodes-view.md
---
-->

# Using the Nodes View

## Introduction

As of v1.15.0 of Netdata, and in conjunction with our announcement post about the [future of Netdata](https://blog.netdata.cloud/posts/netdata-cloud-announcement/), we have enabled an entirely new way to view your infrastructure using the open-source Netdata agent in conjunction with Netdata Cloud: the **Nodes View**. 

This view, powered by Netdata Cloud, provides an aggregated view of the Netdata agents that you have associated with your Netdata Cloud account. The main benefit of Nodes View is seeing the health of your infrastructure from a single interface, especially if you have many systems running Netdata. With Nodes View, you can monitor the health status of your nodes via active alarms and view a subset of real-time performance metrics the agent is collecting every second.

!!! attention "Nodes View is beta software!"
    The Nodes View is currently in beta, so all typical warnings about beta software apply. You may come across bugs or inconsistencies.

```
The current version of Nodes uses the API available on each Netdata agent to check for new alarms and the machine's overall health/availability. In the future, we will offer both polling via the API and real-time streaming of health status/metrics.
```

## The Nodes View

To access the Nodes View, you must first be signed in to Netdata Cloud. To register for an account, or sign in to an existing account, visit our [signing in guide](signing-in.md) for details.

Once you're signed in to Netdata Cloud, clicking on any of the **Nodes Beta** buttons in the node's web dashboard will lead you to the Nodes View. Find one (`1`) in the dropdown menu in the upper-right corner, a second (`2`) in the top navigation bar, and a third (`3`) in the dropdown menu in the top-left corner of the Netdata dashboard.

![Annotated screenshot showing where to access Nodes View](https://user-images.githubusercontent.com/1153921/60359236-4fd04b00-998d-11e9-9e4c-f35ad2551a54.png)

### Nodes

The primary component of the Nodes View is a list of all the nodes with Netdata agents you have associated with your Netdata Cloud account via the Netdata Cloud registry.

![A screenshot of the Netdata Cloud web interface](https://user-images.githubusercontent.com/1153921/59883580-657cb980-936a-11e9-8651-a51832a5f41e.png)

Depending on which [view mode](#view-modes) you're using, Nodes View will present you with information about that node, such as its hostname, operating system, warnings/critical alerts, and any [supported services](#Services-available-in-the-Nodes-View) that are running on that node. Here is an example of the **full** view mode:

![Annotated screenshot of the icons visible in the node entries](https://user-images.githubusercontent.com/1153921/60219761-9eb0a000-9828-11e9-9f77-b492dad016f9.png)

The background color of each Node entry is an indication of its health status:

| Health status | Background color                                                                                  |
| ------------- | ------------------------------------------------------------------------------------------------- |
| **White**     | Normal status, no alarms                                                                          |
| **Yellow**    | 1 or more active warnings                                                                         |
| **Red**       | 1 or more active critical alerts                                                                  |
| **Grey**      | Node is unreachable (server unreachable [due to network conditions], server down, or changed URL) |

### Node overview

When you click on any of the Nodes, an overview sidebar will appear on the right-hand side of the Nodes View.

This overview contains the following:

-   An icon (`1`) representing the operating system installed on that machine
-   The hostname (`2`) of the machine
-   A link (`3`) to the URL at which the web dashboard is available
-   Three tabs (`4`) for **System** metrics, **Services** metrics, and **Alarms**
-   A number of selectors (`5`) to choose which metrics/alarms are shown in the overview
    -   **System** tab: _Overview_, _Disks_, and _Network_ selectors
    -   **Services** tab: _Databases_, _Web_, and _Messaging_ selectors
    -   **Alarms** tab: _Critical_ and _Warning_ selectors
-   The visualizations and/or alarms (`6`) supported under the chosen tab and selector
-   Any other available URLS (`7`) associated with that node under the **Node URLs** header.

![A screenshot of the system overview area in the Netdata Cloud web interface](https://user-images.githubusercontent.com/1153921/60361418-f834de00-9992-11e9-9998-ab3da4b8b559.png)

By default, clicking on a Node will display the sidebar with the **System** tab enabled. If there are warnings or alarms active for that Node, the **Alarms** tab will be displayed by default.

**The visualizations in the overview sidebar are live!** As with all of Netdata's visualizations, you can scrub forward and backward in time, zoom, pause, and pinpoint anomalies down to the second.

#### System tab

The **System** tab has three sections: *Overview*, *Disks*, and *Network*.

_Overview_ displays visualizations for `CPU`, `System Load Average` `Disk I/O`, `System RAM`, `System Swap`, `Physical Network Interfaces Aggregated Bandwidth`, and the URL of the node. 

_Disks_ displays visualizations for `Disk Utilization Time`, and `Disk Space Usage` for every available disk.

_Network_ displays visualizations for `Bandwidth` for every available networking device.

#### Services tab

The **Services** tab will show visualizations for any [supported services](#Services-available-in-the-Nodes-View) that are running on that node. Three selectors are available: _Databases_, _Web_, and _Messaging_. If there are no services under any of these categories, the selector will not be clickable.

#### Alarms tab

The **Alarms** tab contains two selectors: _Critical_ and _Warning_. If there are no alarms under either of these categories, the selector will not be clickable.

Both of these tabs will display alarms information when available, along with the relevant visualization with metrics from your Netdata agent. The `view` link redirects you to the web dashboard for the selected node and automatically shows the appropriate visualization and timeframe.

![A screenshot of the alarms area in the Netdata Cloud web interface](https://user-images.githubusercontent.com/1153921/59883273-55180f00-9369-11e9-8895-f74f6c66e038.png)

### Filtering field

The search field will be useful for Netdata Cloud users with dozens or hundreds of Nodes. You can filter for the hostname of the Node you're interested in, the operating system it's running, or even for the services installed. 

The filtering field will offer you autocomplete suggestions. For example, the options available after typing `ng` into the filtering field:

![A screenshot of the filtering field in the Netdata Cloud web interface](https://user-images.githubusercontent.com/1153921/59883296-6234fe00-9369-11e9-9950-4bd3986ce887.png)

If you select multiple filters, results will display according to an `OR` operator. 

### View modes

To the right of the filtering field is three functions that will help you organize your Visited Nodes according to your preferences.

![Screenshot of the view mode, sorting, and grouping options](https://user-images.githubusercontent.com/1153921/59885999-2a7e8400-9372-11e9-8dae-022ba85e2b69.png)

The view mode button lets you switch between three view modes:

-   **Full** mode, which displays the following information in a large squares for each connected Node:
    -   Operating system
    -   Critical/warning alerts in two separate indicators
    -   Hostname
    -   Icons for [supported services](#services-available-in-the-nodes-view)

![Annotated screenshot of the full view mode](https://user-images.githubusercontent.com/1153921/60219885-15e63400-9829-11e9-8654-b49f119efb9a.png)

-   **Compact** mode, which displays the following information in small squares for each connected Node:
    -   Operating system

![Annotated screenshot of the compact view mode](https://user-images.githubusercontent.com/1153921/60220570-547cee00-982b-11e9-9caf-9dd449184f3a.png)

-   **Detailed** mode, which displays the following information in large horizontal rectangles for each connected Node:
    -   Operating system
    -   Critical/warning alerts in two separate indicators
    -   Hostname
    -   Icons for [supported services](#services-available-in-the-nodes-view) 

![Annotated screenshot of the detailed view mode](https://user-images.githubusercontent.com/1153921/60220574-56df4800-982b-11e9-8300-aa9190bbf09f.png)

## Sorting, and grouping

The **Sort by** dropdown allows you to choose between sorting _alphabetically by hostname_, most _recently-viewed_ nodes, and most _frequently-view_ nodes.

The **Group by** dropdown lets you switch between _alarm status_, _running services_, or _online status_.

For example, the following screenshot represents the Nodes list with the following options: _detailed list_, _frequently visited_, and _alarm status_.

![A screenshot of sorting, grouping, and view modes in the Netdata Cloud web interface](https://user-images.githubusercontent.com/1153921/59883300-68c37580-9369-11e9-8d6e-ce0a8147fc1d.png)

Play around with the options until you find a setup that works for you.

## Adding more agents to the Nodes View

There is currently only one way to associate additional Netdata nodes with your Netdata Cloud account. You must visit the web dashboard for each node and click the **Sign in** button and complete the [sign in process](signing-in.md#signing-in-to-your-netdata-cloud-account).

!!! note ""
    We are aware that the process of registering each node individually is cumbersome for those who want to implement Netdata Cloud's features across a large infrastructure. 

```
Please view [this comment on issue #6318](https://github.com/netdata/netdata/issues/6318#issuecomment-504106329) for how we plan on improving the process for adding additional nodes to your Netdata Cloud account.
```

## Services available in the Nodes View

The following tables elaborate on which services will appear in the Nodes View. Alerts from [other collectors](../../collectors/README.md), when entered an alarm status, will show up in the _Alarms_ tab despite not appearing 

### Databases

These services will appear under the _Databases_ selector beneath the _Services_ tab.

| Service  	      | Collectors  	                | Context #1  	| Context #2  	| Context #3   	|
|---	|---	|---	|---	|---	|
| MySQL  	      | `python.d.plugin:mysql`, `go.d.plugin:mysql`  	| `mysql.queries`  	|  `mysql.net` 	| `mysql.connections`   	|
| MariaDB  	      | `python.d.plugin:mysql`, `go.d.plugin:mysql`  	| `mysql.queries`  	| `mysql.net`  	| `mysql.connections`  	|
| Oracle Database | `python.d.plugin:oracledb`  	| `oracledb.session_count`  	| `oracledb.physical_disk_read_writes ` 	| `oracledb.tablespace_usage_in_percent`   	|
| PostgreSQL  	  | `python.d.plugin:postgres`  	| `postgres.checkpointer`  	| `postgres.archive_wal`  	| `postgres.db_size`  	|
| MongoDB  	      | `python.d.plugin:mongodb`  	    | `mongodb.active_clients`  	| `mongodb.read_operations`  	| `mongodb.write_operations`  	|
| ElasticSearch   | `python.d.plugin:elasticsearch` | `elastic.search_performance_total`  	| `elastic.index_performance_total`  	| `elastic.index_segments_memory`  	|
| CouchDB  	      | `python.d.plugin:couchdb`  	    | `couchdb.activity`  	| `couchdb.response_codes`  	|   	|
| Proxy SQL  	  | `python.d.plugin:proxysql`  	| `proxysql.questions`  	| `proxysql.pool_status`  	| `proxysql.pool_overall_net`  	|
| Redis  	      | `python.d.plugin:redis`  	    | `redis.operations`  	| `redis.net`  	| `redis.connections`  	|
| MemCached  	  | `python.d.plugin:memcached`  	| `memcached.cache`  	| `memcached.net`  	| `memcached.connections`  	|
| RethinkDB  	  | `python.d.plugin:rethinkdbs`  	| `rethinkdb.cluster_queries`  	| `rethinkdb.cluster_clients_active`  	| `rethinkdb.cluster_connected_servers`  	|
| Solr  	      | `go.d.plugin:solr`  	        | `solr.search_requests`  	| `solr.update_requests`  	|   	|

### Web services

These services will appear under the _Web_ selector beneath the _Services_ tab. These also include proxies, load balancers (LB), and streaming services.

| Service  	| Collectors  	| Context #1  	| Context #2  	| Context #3   	|
|---	|---	|---	|---	|---	|
| Apache  	| `python.d.plugin:apache`, `go.d.plugin:apache`  	|  `apache.requests`	| `apache.connections`  	| `apache.net ` 	|
| nginx  	| `python.d.plugin:nginx`, `go.d.plugin:nginx`  	| `nginx.requests`  	| `nginx.connections`  	|   	|
| nginx+  	| `python.d.plugin:nginx_plus`  	| `nginx_plus.requests_total`  	| `nginx_plus.connections_statistics`  	|   	|
| lighthttpd  	| `python.d.plugin:lighttpd`, `go.d.plugin:lighttpd`  	| `lighttpd.requests`  	| `lighttpd.net`  	|   	|
| lighthttpd2  	| `go.d.plugin:lighttpd2`  	| `lighttpd2.requests`  	| `lighttpd2.traffic`  	|   	|
| LiteSpeed  	| `python.d.plugin:litespeed`  	| `litespeed.requests`  	| `litespeed.requests_processing`  	|   	|
| Tomcat  	| `python.d.plugin:tomcat`  	| `tomcat.accesses`  	| `tomcat.processing_time`  	| `tomcat.bandwidth`  	|
| PHP FPM  	| `python.d.plugin:phpfm`  	| `phpfpm.performance` 	| `phpfpm.requests`  	| `phpfpm.connections`  	|
| HAproxy  	| `python.d.plugin:haproxy`  	| `haproxy_f.scur`  	| `haproxy_f.bin`  	| `haproxy_f.bout`  	|
| Squid  	| `python.d.plugin:squid`  	| `squid.clients_requests`  	| `squid.clients_net`  	|   	|
| Traefik  	| `python.d.plugin:traefik`  	| `traefik.response_codes`  	|   	|   	|
| Varnish  	| `python.d.plugin:varnish`  	| `varnish.session_connection`  	| `varnish.client_requests`  	|   	|
| IPVS  	| `proc.plugin:/proc/net/ip_vs_stats`  	| `ipvs.sockets`  	| `ipvs.packets`  	|   	|
| Web Log  	| `python.d.plugin:web_log`, `go.d.plugin:web_log`  	| `web_log.response_codes`  	| `web_log.bandwidth`  	|   	|
| IPFS  	| `python.d.plugin:ipfs`  	| `ipfs.bandwidth`  	|  `ipfs.peers` 	|   	|
| IceCast Media Streaming | `python.d.plugin:icecast`  	| `icecast.listeners`  	|   	|   	|
| RetroShare  	          | `python.d.plugin:retroshare`  	| `retroshare.bandwidth`  	| `retroshare.peers`  	|   	|
| HTTP Check  	          |  `python.d.plugin:httpcheck`, `go.d.plugin:httpcheck` 	| `httpcheck.responsetime`  	| `httpcheck.status`  	|   	|
| x509 Check  	          | `go.d.plugin:x509check`  	| `x509check.time_until_expiration`  	|   	|   	|

### Messaging

These services will appear under the _Messaging_ selector beneath the _Services_ tab.

| Service  	 | Collectors  	                                      | Context #1  	            | Context #2  	               | Context #3   	           |
| ---	| ---	| ---	| ---	| ---	|
| RabbitMQ   | `python.d.plugin:rabbitmq`, `go.d.plugin:rabbitmq` | `rabbitmq.queued_messages`  | `rabbitmq.erlang_run_queue`  |            
| Beanstalkd | `python.d.plugin:beanstalk`  	                  | `beanstalk.total_jobs_rate` | `beanstalk.connections_rate` | `beanstalk.current_tubes` |

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fdocs%2Fnetdata-cloud%2Fnodes-view&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
