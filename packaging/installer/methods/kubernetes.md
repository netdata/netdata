<!--
title: "Deploy Kubernetes monitoring with Netdata"
description: "Deploy Netdata to monitor a Kubernetes cluster to monitor the health, performance, resource utilization, and application metrics of a Kubernetes cluster in real time."
custom_edit_url: https://github.com/netdata/netdata/edit/master/packaging/installer/methods/kubernetes.md
-->

# Deploy Kubernetes monitoring with Netdata

This document details how to install Netdata on an existing Kubernetes (k8s) cluster. By following these directions, you
will use Netdata's [Helm chart](https://github.com/netdata/helmchart) to create a Kubernetes monitoring deployment on
your cluster.

The Helm chart installs one `parent` pod for storing metrics and managing alarm notifications, plus an additional
`child` pod for every node in the cluster, responsible for collecting metrics from the node, Kubernetes control planes,
pods/containers, and [supported application-specific
metrics](https://github.com/netdata/helmchart#service-discovery-and-supported-services).

To deploy Kubernetes monitoring with Netdata, you need:

-   A working cluster running Kubernetes v1.9 or newer.
-   The [kubectl](https://kubernetes.io/docs/reference/kubectl/overview/) command line tool, within [one minor version
    difference](https://kubernetes.io/docs/tasks/tools/install-kubectl/#before-you-begin) of your cluster, on an
    administrative system.
-   The [Helm package manager](https://helm.sh/) v3.0.0 or newer on the same administrative system.

## Install the Netdata Helm chart

We recommend you install the Helm chart using our Helm repository. In the `helm install` command, replace `netdata` with
the release name of your choice.

```bash
helm repo add netdata https://netdata.github.io/helmchart/
helm install netdata netdata/netdata
```

Run `kubectl get services` and `kubectl get pods` to confirm that your cluster now runs a `netdata` service, one
parent pod, and multiple child pods.

You've now installed Netdata on your Kubernetes cluster. Next, it's time to opt-in and enable the powerful Kubernetes
dashboards available in Netdata Cloud.

## Connect your Kubernetes cluster to Netdata Cloud

To start [Kubernetes monitoring](https://learn.netdata.cloud/docs/cloud/visualize/kubernetes/), you must first
[connect](/claim/README.md) your Kubernetes cluster to [Netdata Cloud](https://app.netdata.cloud). The connection process securely
connects your Kubernetes cluster to stream metrics data to Netdata Cloud, enabling Kubernetes-specific visualizations
like the health map and time-series composite charts.

### New installations

First, find the script to run an `helm install` command. You can get it by clicking on your Space's dropdown, then **Manage your Space**.
Click the **Nodes** tab and select the environment your node is running, in this case **kubernetes**, to reveal the script for your Space in Netdata Cloud. You need the `TOKEN`
and `ROOM` values.

The script should be similar to:

```bash
helm install netdata netdata/netdata --set parent.claiming.enabled="true" --set parent.claiming.token="TOKEN" --set parent.claiming.rooms="ROOM" --set child.claiming.enabled=true --set child.claiming.token="TOKEN" --set child.claiming.rooms="ROOM"
```

### Existing installations

On an existing installation, you will need to override the configuration values by running the `helm upgrade` command and provide a file with the values to override. You can start with creating a file called `override.yml`.

```bash
touch override.yml
```

Paste the following into your `override.yml` file, replacing instances of `ROOM` and `TOKEN` with those from the script from Netdata Cloud. These settings connect your `parent`/`child` nodes to Netdata Cloud and store more
metrics in the nodes' time-series databases.

```yaml
parent:
  claiming:
    enabled: true
    token: "TOKEN"
    rooms: "ROOM"

child:
  claiming:
    enabled: true
    token: "TOKEN"
    rooms: "ROOM"
  configs:
    netdata:
      data: |
        [global]
          memory mode = ram
          history = 3600
        [health]
          enabled = no
```

> ❗ These override settings, along with the Helm chart's defaults, will retain an hour's worth of metrics (`history =
> 3600`, or `3600 seconds`) on each child node. Based on your metrics retention needs, and the resources available on
> your cluster, you may want to increase the `history` setting.

Apply these new settings:

```bash
helm upgrade -f override.yml netdata netdata/netdata
```

The cluster terminates the old pods and creates new ones with the proper persistence and connection configuration. You'll
see your nodes, containers, and pods appear in Netdata Cloud in a few seconds.

![Netdata's Kubernetes monitoring
visualizations](https://user-images.githubusercontent.com/1153921/107801491-5dcb0f00-6d1d-11eb-9ab1-876c39f556e2.png)

If you don't need to configure your Netdata deployment, [skip down](#whats-next) to see how Kubernetes monitoring works
in Netdata, in addition to more guides and resources.

## Configure your Netdata monitoring deployment

Read up on the various configuration options in the [Helm chart
documentation](https://github.com/netdata/helmchart#configuration) if you need to tweak your Kubernetes monitoring.

Your first option is to create an `override.yml` file, if you haven't created one already for
[connect](#connect-your-kubernetes-cluster-to-netdata-cloud), then apply the new configuration to your cluster with `helm
upgrade`.

```bash
helm upgrade -f override.yml netdata netdata/netdata
```

If you want to change only a single setting, use the `--set` argument with `helm upgrade`. For example, to change the
size of the persistent metrics volume on the parent node:

```bash
helm upgrade --set parent.database.volumesize=4Gi netdata netdata/netdata
```

### Configure service discovery

Netdata's [service discovery](https://github.com/netdata/agent-service-discovery/#service-discovery), installed as part
of the Helm chart installation, finds what services are running in a cluster's containers and automatically collects
service-level metrics from them.

Service discovery supports [popular applications](https://github.com/netdata/helmchart#applications) and [Prometheus
endpoints](https://github.com/netdata/helmchart#prometheus-endpoints).

If your cluster runs services on non-default ports or uses non-default names, you may need to configure service
discovery to start collecting metrics from your services. You have to edit the default ConfigMap that is shipped with
the Helmchart and deploy that to your cluster.

First, copy the default file to your administrative system.

```bash
curl https://raw.githubusercontent.com/netdata/helmchart/master/charts/netdata/sdconfig/child.yml -o child.yml
```

Edit the new `child.yml` file according to your needs. See the [Helm chart
configuration](https://github.com/netdata/helmchart#configuration) and the file itself for details.

You can then run `helm upgrade` with the `--set-file` argument to use your configured `child.yml` file instead of the
default, changing the path if you copied it elsewhere.

```bash
helm upgrade --set-file sd.child.configmap.from.value=./child.yml netdata netdata/netdata
```

Now that you pushed an edited ConfigMap to your cluster, service discovery should find and set up metrics collection
from your non-default service.

## Update/reinstall the Netdata Helm chart

If you update the Helm chart's configuration, run `helm upgrade` to redeploy your Netdata service, replacing `netdata`
with the name of the release, if you changed it upon installation:

```bash
helm upgrade netdata netdata/netdata
```

To update Netdata's Helm chart to the latest version, run `helm repo update`, then deploy `upgrade` it`:

```bash
helm repo update
helm upgrade netdata netdata/netdata
```

## What's next?

[Start Kubernetes monitoring](https://learn.netdata.cloud/docs/cloud/visualize/kubernetes/) in Netdata Cloud, which
comes with meaningful visualizations out of the box. 

Read our guide, [_Kubernetes monitoring with Netdata: Overview and
visualizations_](/docs/guides/monitor/kubernetes-k8s-netdata.md), for a complete walkthrough of Netdata's Kubernetes
monitoring capabilities, including a health map of every container in your infrastructure, aggregated resource
utilization metrics, and application metrics.

### Related reference documentation

- [Netdata Cloud · Kubernetes monitoring](https://learn.netdata.cloud/docs/cloud/visualize/kubernetes/)
- [Netdata Helm chart](https://github.com/netdata/helmchart)
- [Netdata service discovery](https://github.com/netdata/agent-service-discovery/)

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fpackaging%2Finstaller%2Fmethods%2Fkubernetes&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
