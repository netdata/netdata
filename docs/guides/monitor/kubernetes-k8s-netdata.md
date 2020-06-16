<!--
title: "Monitor a Kubernetes (k8s) cluster with Netdata"
description: "Use Netdata's helmchart, service discovery plugin, and Kubelet/kube-proxy collectors for real-time visibility into your Kubernetes cluster."
image: /img/seo/guides/monitor-kubernetes-k8s-netdata.png
-->

# Monitor a Kubernetes cluster with Netdata

**ADD AN INTRO**

We offer four main methods of monitoring a Kubernetes cluster:

-   A [Helm chart](https://github.com/netdata/helmchart), which bootstraps a Netdata deployment on a Kubernetes cluster.
-   A [service discovery plugin](https://github.com/netdata/agent-service-discovery), which discovers 22 different
    services that might be running inside of your cluster's pods and creates the configuration files required for
    Netdata to monitor that service. Compatible services include Nginx, Apache, MySQL, CoreDNS, and much more.
-   A [Kubelet collector](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/k8s_kubelet), which runs
    on each node in a k8s cluster to monitor the number of pods/containers, the volume of operations on each container,
    and more.
-   A [kube-proxy collector](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/k8s_kubeproxy), which
    also runs on each node and monitors latency and the volume of HTTP requests to the proxy.

These four methods work together to help you troubleshoot performance or availablility issues across your Kubernetes
infrastructure.

Let's get started.

## Prerequisites

To follow this guide, you need a working k8s cluster running Kubernetes v1.9 or newer, the
[kubectl](https://kubernetes.io/docs/reference/kubectl/overview/) command line tool on the system you use to administer
your k8s cluster, and the [Helm package manager](https://helm.sh/) on that same system.

This guide uses a 3-node cluster, running on Digital Ocean, as an example. This cluster also runs CockroachDB, which
we'll use as an example of how to monitor not only the health of Kubernetes cluster itself, but also the service(s)
running in its pods.

```bash
kubectl get nodes 
NAME                   STATUS   ROLES    AGE   VERSION
pool-0z7557lfb-3fnbf   Ready    <none>   51m   v1.17.5
pool-0z7557lfb-3fnbx   Ready    <none>   51m   v1.17.5
pool-0z7557lfb-3fnby   Ready    <none>   51m   v1.17.5

kubectl get pods
NAME                            READY   STATUS      RESTARTS   AGE
cockroachdb-0                   1/1     Running     0          92m
cockroachdb-1                   1/1     Running     0          92m
cockroachdb-2                   1/1     Running     1          92m
cockroachdb-init-q7mp6          0/1     Completed   0          92m
```

Once you're ready, you can start deploying Netdata monitoring on your k8s cluster.

## Install the Netdata Helm chart

To monitor a Kubernetes cluster with Netdata, you need to begin by installing the Netdata Helm chart.

Download the [Netdata Helm chart](https://github.com/netdata/helmchart) on the administation system where you have the
`helm` binary installed.

```bash
git clone https://github.com/netdata/helmchart.git netdata-helmchart
```

You may not need to configure either the Helm chart or the service discovery configuration file, but it's important to
read up on the following two steps to better understand how they work.

### Configure the Helm chart for your cluster

Read up on the various configuration options in the [Helm chart
documentation](https://github.com/netdata/helmchart#configuration) to see if you need to change any of the options based
on your cluster's setup.

For example, Digital Ocean's Kubernetes implementation requires that volumes are a minimum of 1Gi, and the default
setting for the parent pod's persistent storage for alarms is 100Mi (`parent.alarms.volumesize`).

To override a setting, you can edit the `values.yml` file inside of the `netdata-helmchart` folder, or you can use
`--set`: `--set parent.alarms.volumesize=1Gi` when running `helm install`.

### Configure service discovery

As mentioned in the introduction, Netdata has a [service discovery
plugin](https://github.com/netdata/agent-service-discovery/#service-discovery) to identify compatible pods and collect
metrics from the service they run. This service discovery plugin is installed into your k8s cluster when you use the
Netdata Helm chart.

Service discovery scans your cluster for pods exposed on certain ports and with certain image names. By default, it
looks for its [supported services](https://github.com/netdata/helmchart/blob/master/sd-slave.yml#L11-L54) on the ports
they most commonly listen on, and using default image names.

For example, this example Kubernetes cluser is running CockroachDB. The service discovery plugin looks for images with a
name following the pattern `**/cockroach*` and running on port `8080`.

If you haven't changed listening ports or other defaults, service discovery should find your pods and the services
running inside of them, create the proper configurations, and begin monitoring them as soon as the Netdata pods finish
deploying.

However, if you have changed some of these defaults, and want to monitor



### Install the Helm chart

If you didn't configure your cluster in the previous step, you can run the `helm` command below, replacing `--name
netdata` with the release name of your choosing.

```bash
helm install --name netdata ./netdata-helmchart
```

> If you edited the `values.yml` file in the previous step, add `-f netdata-helmchart/values.yaml` to the end of the
> above command: `helm install --name netdata ./netdata-helmchart -f netdata-helmchart/values.yaml`.

Run `kubectl get services` and `kubectl get pods` to confirm the `netdata` service, plus the various `parent` and
`child` pods, were created successfully. Your output should look similar to the following.

You now have a deployment of Netdata pods to help you monitor your Kubernetes cluster. The `netdata-child` pods collect
metrics and stream them to `netdata-parent`, which uses two persistent volumes to store metrics and alarms. In addition,
the parent pod handles alarm notifications and enables the Netdata dashboard using an ingress controller.

### Update/reinstall or remove the Netdata Helm chart

If you want to update the Helm chart's configuration, you can run `helm upgrade`, replacing `netdata` with the name of
the release if you changed it upon installtion.

```bash
helm upgrade netdata ./netdata-helmchart
```

Delete the Helm deployment altogether with the following command:

```bash
helm delete netdata --purge 
```

Based on your k8s provider, you may need to manually delete volumes or volumes claims.

## Start monitoring your Kubernetes cluster with Netdata

The Helm chart installs and enables everything you need for visibility into your k8s cluster, including the service
discovery plugin, Kubelet collector, and kube-proxy collector.

The service discovery plugin itself

### Explore pod metrics

- See the K
- cgroups collector shows the various containers running in each 

### Explore service discovery

t/k

### Explore Kubelet charts

t/k

### Explore kube-proxy charts

t/k

### See all other containers

## What's next?

We're continuing to develop Netdata's k8s/container monitoring capabilities, but for now, this setup allows you to
better

Platform for troubleshooting performance and availability issues

work efficiently to identify the root cause of issues



[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fdocs%2Fguides%2Fmonitor%2Fkubernetes-k8s-netdata.md&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
