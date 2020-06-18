<!--
title: "Monitor a Kubernetes (k8s) cluster with Netdata"
description: "Use Netdata's helmchart, service discovery plugin, and Kubelet/kube-proxy collectors for real-time visibility into your Kubernetes cluster."
image: /img/seo/guides/monitor-kubernetes-k8s-netdata.png
-->

# Monitor a Kubernetes cluster with Netdata

**ADD AN INTRO**

We offer a few complimentary methods of monitoring a Kubernetes cluster:

-   A [Helm chart](https://github.com/netdata/helmchart), which bootstraps a Netdata Agent pod on every node in your
    cluster, plus an additional parent pod for storing metrics and managing alarm notifications.
-   A [service discovery plugin](https://github.com/netdata/agent-service-discovery), which discovers 22 different
    services that might be running inside of your cluster's pods and creates the configuration files required for
    Netdata to monitor that service. [Compatible services]() include Nginx, Apache, MySQL, CoreDNS, and much more.
-   A [Kubelet collector](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/k8s_kubelet), which runs
    on each node in a k8s cluster to monitor the number of pods/containers, the volume of operations on each container,
    and more.
-   A [kube-proxy collector](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/k8s_kubeproxy), which
    also runs on each node and monitors latency and the volume of HTTP requests to the proxy.
-   A [cgroups collector](/collectors/cgroups.plugin/README.md), which 

These methods work together to help you troubleshoot performance or availablility issues across your Kubernetes
infrastructure.

Let's get started.

## Prerequisites

To follow this guide, you need:

-   A working k8s cluster running Kubernetes v1.9 or newer.
-   The [Helm package manager](https://helm.sh/) on an administrative system.
-   The [kubectl](https://kubernetes.io/docs/reference/kubectl/overview/) command line tool on the same system.

**You all need to install the Netdata Helm chart on your cluster** before you proceed. See our [Kubernetes installation
process](/packaging/installer/methods/kubernetes.md) for details.

This guide uses a 3-node cluster, running on Digital Ocean, as an example. This cluster runs CockroachDB, Redis, and
Apache, which we'll use as examples of how to monitor not only the cluster itself, but also the services running in its
many pods.

```bash
kubectl get nodes 
NAME                   STATUS   ROLES    AGE   VERSION
pool-0z7557lfb-3fnbf   Ready    <none>   51m   v1.17.5
pool-0z7557lfb-3fnbx   Ready    <none>   51m   v1.17.5
pool-0z7557lfb-3fnby   Ready    <none>   51m   v1.17.5

kubectl get pods
NAME                     READY   STATUS      RESTARTS   AGE
cockroachdb-0            1/1     Running     0          44h
cockroachdb-1            1/1     Running     0          44h
cockroachdb-2            1/1     Running     1          44h
cockroachdb-init-q7mp6   0/1     Completed   0          44h
httpd-6f6cb96d77-4zlc9   1/1     Running     0          2m47s
httpd-6f6cb96d77-d9gs6   1/1     Running     0          2m47s
httpd-6f6cb96d77-xtpwn   1/1     Running     0          11m
netdata-child-5p2m9      2/2     Running     0          42h
netdata-child-92qvf      2/2     Running     0          42h
netdata-child-djc6w      2/2     Running     0          42h
netdata-parent-0         1/1     Running     0          42h
redis-6bb94d4689-6nn6v   1/1     Running     0          73s
redis-6bb94d4689-c2fk2   1/1     Running     0          73s
redis-6bb94d4689-tjcz5   1/1     Running     0          88s
```

## Explore Netdata's Kubernetes charts

The Helm chart installs and enables everything you need for visibility into your k8s cluster, including the service
discovery plugin, Kubelet collector, and kube-proxy collector.

The service discovery plugin itself

### Pods

- See the K
- cgroups collector shows the various containers running in each 

### Applications running inside of pods (service discovery)

t/k

### Kubelet

t/k

### kube-proxy

t/k

### Containers via cgroups

t/k

## What's next?

We're continuing to develop Netdata's k8s/container monitoring capabilities, but for now, this setup allows you to
better

Platform for troubleshooting performance and availability issues

work efficiently to identify the root cause of issues



[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fdocs%2Fguides%2Fmonitor%2Fkubernetes-k8s-netdata.md&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
