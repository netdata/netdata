<!--
title: "Deploy Netdata in a Kubernetes Cluster"
sidebar_label: "Deploy Netdata in a Kubernetes Cluster"
custom_edit_url: "https://github.com/netdata/netdata/blob/master/docs/getting-started/installation/deploy-netdata-in-a-kubernetes-cluster.md"
sidebar_position: "13"
learn_status: "Unpublished"
learn_topic_type: "Getting started"
learn_rel_path: "Getting started/Installation"
learn_docs_purpose: "Instructions on deploying Netdata in a Kubernetes cluster"
-->

This document details how to install Netdata on an existing Kubernetes (k8s) cluster. By following these directions, you
will use Netdata's [Helm chart](https://github.com/netdata/helmchart) to create a Kubernetes monitoring deployment on
your cluster.

The Helm chart installs one `parent` pod for storing metrics and managing alarm notifications, plus an additional
`child` pod for every node in the cluster, responsible for collecting metrics from the node, Kubernetes control planes,
pods/containers, and [supported application-specific
metrics](https://github.com/netdata/helmchart#service-discovery-and-supported-services).

## Prerequisites

To deploy Kubernetes monitoring with Netdata, you need:

- A working cluster running Kubernetes v1.9 or newer.
- The [kubectl](https://kubernetes.io/docs/reference/kubectl/overview/) command line tool, within [one minor version
  difference](https://kubernetes.io/docs/tasks/tools/install-kubectl/#before-you-begin) of your cluster, on an
  administrative system.
- The [Helm package manager](https://helm.sh/) v3.0.0 or newer on the same administrative system.

## Steps

We recommend you install the Helm chart using our Helm repository. In the `helm install` command, replace `netdata` with
the release name of your choice.

1. ```bash
    helm repo add netdata https://netdata.github.io/helmchart/
    helm install netdata netdata/netdata
    ```

2. Run `kubectl get services` and `kubectl get pods` to confirm that your cluster now runs a `netdata` service, one
   parent pod, and multiple child pods.

You've now installed Netdata on your Kubernetes cluster. Next, it's time to enable the powerful Kubernetes
dashboards available in Netdata Cloud! To do so check out our Task
on [claiming an Agent to the Cloud](https://github.com/netdata/netdata/blob/master/docs/tasks/general-configuration/claim-an-agent-to-the-hub.md)
.