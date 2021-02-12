<!--
title: "Deploy Kubernetes monitoring with Netdata"
description: "Install Netdata to monitor a Kubernetes cluster to monitor the health, performance, and resource utilization of a Kubernetes deployment in real time."
custom_edit_url: https://github.com/netdata/netdata/edit/master/packaging/installer/methods/kubernetes.md
-->

# Install Netdata on a Kubernetes cluster

This document details how to install Netdata on an existing Kubernetes (k8s) cluster. By following these directions, you
will use Netdata's [Helm chart](https://github.com/netdata/helmchart) to bootstrap a Netdata deployment on your cluster.
The Helm chart installs one parent pod for storing metrics and managing alarm notifications plus an additional child pod
for every node in the cluster.

Each child pod will collect metrics from the node it runs on, in addition to [compatible
applications](https://github.com/netdata/helmchart#service-discovery-and-supported-services), plus any endpoints covered
by our [generic Prometheus collector](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/prometheus),
via [service discovery](https://github.com/netdata/agent-service-discovery/). Each child pod will also collect
[cgroups](/collectors/cgroups.plugin/README.md),
[Kubelet](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/k8s_kubelet), and
[kube-proxy](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/k8s_kubeproxy) metrics from its node.

To install Netdata on a Kubernetes cluster, you need:

-   A working cluster running Kubernetes v1.9 or newer.
-   The [kubectl](https://kubernetes.io/docs/reference/kubectl/overview/) command line tool, within [one minor version
    difference](https://kubernetes.io/docs/tasks/tools/install-kubectl/#before-you-begin) of your cluster, on an
    administrative system.
-   The [Helm package manager](https://helm.sh/) v3.0.0 or newer on the same administrative system.

The default configuration creates one `parent` pod, installed on one of your cluster's nodes, and a DaemonSet for
additional `child` pods. This DaemonSet ensures that every node in your k8s cluster also runs a `child` pod, including
the node that also runs `parent`. The `child` pods collect metrics and stream the information to the `parent` pod, which
uses two persistent volumes to store metrics and alarms. The `parent` pod also handles alarm notifications and enables
the Netdata dashboard using an ingress controller.

## Install the Netdata Helm chart

We recommend you install the Helm chart using our Helm repository. In the `helm install` command, replace `netdata` with
the release name of your choice.

```bash
helm repo add netdata https://netdata.github.io/helmchart/
helm install netdata netdata/netdata
```

> You can also install the Netdata Helm chart by cloning the
> [repository](https://artifacthub.io/packages/helm/netdata/netdata#install-by-cloning-the-repository) and manually
> running Helm against the included chart.

### Post-installation

Run `kubectl get services` and `kubectl get pods` to confirm that your cluster now runs a `netdata` service, one
parent pod, and multiple child pods.

Take note of the name of the parent pod, which will look like: `netdata-parent-xxxxxxxxx-xxxxx`.

You've now installed Netdata on your Kubernetes cluster. Next, it's time to enable the powerful Kubernetes dashboards
available in Netdata Cloud.

## Claim your Kubernetes cluster to Netdata Cloud

[Claim](/claim/README.md) your Kubernetes cluster to stream metadata for monitoring. Claiming securely connects your
node to [Netdata Cloud](https://app.netdata.cloud) to create visualizations and alerts. 

Ensure persistence is enabled on the parent pod by running the following `helm upgrade` command.

```bash
helm upgrade \
  --set parent.database.persistence=true \
  --set parent.alarms.persistence=true \
  netdata netdata/netdata
```

Next, find your claiming script in Netdata Cloud by clicking on your Space's dropdown, then **Manage your Space**. Click
the **Nodes** tab. Netdata Cloud shows a script similar to the following:

```bash
sudo netdata-claim.sh -token=TOKEN -rooms=ROOM1,ROOM2 -url=https://app.netdata.cloud
```

You will need the values of `TOKEN` and `ROOM1,ROOM2` for the command, which sets `parent.claiming.enabled`,
`parent.claiming.token`, and `parent.claiming.rooms` to complete the parent pod claiming process.

Run the following `helm upgrade` command after replacing `TOKEN` and `ROOM1` with the values found in the claiming
script from Netdata Cloud. The quotations are required.

```bash
helm upgrade \
  --set parent.claiming.enabled=true \
  --set parent.claiming.token="TOKEN" \
  --set parent.claiming.rooms="ROOM" \
  netdata netdata/netdata
```

The cluster terminates the old parent pod and creates a new one with the proper claiming configuration. You'll see your
parent pod appear in Netdata Cloud in a few seconds.

![Netdata's Kubernetes monitoring
visualizations](https://user-images.githubusercontent.com/1153921/107801491-5dcb0f00-6d1d-11eb-9ab1-876c39f556e2.png)

### Claim child pods (optional)

It's possible to claim child pods to Netdata Cloud to visualize all available metrics from both the Kubernetes cluster
itself _and_ any running applications.

Child pod claiming comes with one large caveat: Because the child pods have no persistent storage, they are re-claimed
under a new GUID every time they are restarted. Depending on how often this happens, this can create many nodes marked
**unreachable** that [cannot be removed]().

Run another `helm upgrade` command, replacing `TOKEN` and `ROOM1` with the same values from the claiming script.

```bash
helm upgrade \
  --set child.claiming.enabled=true \
  --set child.claiming.token="TOKEN" \
  --set child.claiming.rooms="ROOM" \
  netdata netdata/netdata
```

You'll see your child nodes appear in a few seconds.

## Configure your Netdata deployment

Read up on the various configuration options in the [Helm chart
documentation](https://github.com/netdata/helmchart#configuration) to see if you need to change any of the options based
on your cluster's setup.

To change a setting, use the `--set` or `--values` arguments with `helm install`, for the initial deployment, or `helm upgrade` to upgrade an existing deployment. 

```bash
helm install --set a.b.c=xyz netdata netdata/netdata
helm upgrade --set a.b.c=xyz netdata netdata/netdata
```

For example, to change the size of the persistent metrics volume on the parent node:

```bash
helm install --set parent.database.volumesize=4Gi netdata netdata/netdata
helm upgrade --set parent.database.volumesize=4Gi netdata netdata/netdata
```

### Configure service discovery

As mentioned in the introduction, Netdata has a [service discovery
plugin](https://github.com/netdata/agent-service-discovery/#service-discovery) to identify compatible pods and collect
metrics from the service they run. The Netdata Helm chart installs this service discovery plugin into your k8s cluster.

Service discovery scans your cluster for pods exposed on certain ports and with certain image names. By default, it
looks for its supported services on the ports they most commonly listen on, and using default image names. Service
discovery currently supports [popular
applications](https://github.com/netdata/helmchart#service-discovery-and-supported-services), plus any endpoints covered
by our [generic Prometheus collector](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/prometheus).

If you haven't changed listening ports, image names, or other defaults, service discovery should find your pods, create
the proper configurations based on the service that pod runs, and begin monitoring them immediately after deployment.

However, if you have changed some of these defaults, you need to copy a file from the Netdata Helm chart repository,
make your edits, and pass the changed file to `helm install`/`helm upgrade`.

First, copy the file to your administrative system.

```bash
curl https://raw.githubusercontent.com/netdata/helmchart/master/charts/netdata/sdconfig/child.yml -o child.yml
```

Edit the new `child.yml` file according to your needs. See the [Helm chart
configuration](https://github.com/netdata/helmchart#configuration) and the file itself for details. 

You can then run `helm install`/`helm upgrade` with the `--set-file` argument to use your configured `child.yml` file
instead of the default, changing the path if you copied it elsewhere.

```bash
helm install --set-file sd.child.configmap.from.value=./child.yml netdata netdata/netdata
helm upgrade --set-file sd.child.configmap.from.value=./child.yml netdata netdata/netdata
```

Your configured service discovery is now pushed to your cluster.

## Update/reinstall the Netdata Helm chart

If you update the Helm chart's configuration, run `helm upgrade` to redeploy your Netdata service, replacing `netdata`
with the name of the release, if you changed it upon installation:

```bash
helm upgrade netdata netdata/netdata
```

## What's next?

Read the [monitoring a Kubernetes cluster guide](/docs/guides/monitor/kubernetes-k8s-netdata.md) for details on the
various metrics and charts created by the Helm chart and some best practices on real-time troubleshooting using Netdata.

Netdata Cloud features per-container and per-pod visualizations and composite charts using aggregated CPU, memory, disk,
and networking metrics from every later of your cluster. To learn more, see our [Kubernetes
visualization](https://learn.netdata.cloud/docs/cloud/visualizations/kubernetes/) reference doc.

To further configure Netdata for your cluster, see our [Helm chart repository](https://github.com/netdata/helmchart) and
the [service discovery repository](https://github.com/netdata/agent-service-discovery/).

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fpackaging%2Finstaller%2Fmethods%2Fkubernetes&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
