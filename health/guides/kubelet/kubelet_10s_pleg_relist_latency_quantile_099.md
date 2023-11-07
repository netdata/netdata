# kubelet_10s_pleg_relist_latency_quantile_099

**Kubernetes | Kubelet**

_The kubelet is the primary "node agent" that runs on each node. It makes sure that containers are
running in a Pod. The kubelet takes a set of PodSpecs that are provided through various mechanisms
and ensures that the containers described in those PodSpecs are running and healthy and doesn't
manage containers that were not created by Kubernetes._

The PLEG (Pod Lifecycle Event Generator) module in Kubelet adjusts the container runtime state with
each matched pod-level event and keeps the Pod's cache up to date. Big delays in the relist process
of pods will eventually cause a "PLEG is not healthy" event which will make the node unavailable (
NotReady).

The Netadata Agent calculates the ratio of average Pod Lifecycle Event Generator relisting latency
over the last 10 seconds, compared to the last minute (quantile 0.99). Receiving this alert means
that the relisting time has increased significantly.

> Different pods have different relisting latencies, more quantiles help you reduce the error rate in those metrics.


<details>
<summary>See more about the kubelet </summary>

As we said before, the kubelet works in terms of a PodSpec. A PodSpec is a YAML or a JSON object
that describes a pod. The PodSpec contains all information a kubelet needs to know to run the pod in
the corresponding cluster node.

Beside the PodSpecs provided from the Kubernetes APIserver, there are three ways to provide a
kubelet with a container manifest:

- File: Path passed as a flag on the command line. Files under this path will be monitored
  periodically for updates. The monitoring period is 20s by default and is configurable via a flag.
- HTTP endpoint: HTTP endpoint passed as a parameter on the command line. This endpoint is checked
  every 20 seconds (also configurable with a flag).
- HTTP server: The kubelet can also listen for HTTP and respond to a simple API (underspec'd
  currently) to submit a new manifest.

See more about Kubelet in
the [Kubernetes official docs](https://kubernetes.io/docs/reference/command-line-tools-reference/kubelet/)

</details>


<details>
<summary>See more about PLEG and the relist process</summary>

A kubelet keeps track of all the Pods that are about to run in the node. The node could have any
kind of Container Runtime Interface (CRI) always compatible with Kubernetes. A Pod lifecycle event
interprets the underlying container state change at the pod-level abstraction, making it
container-runtime-agnostic. This abstraction shields a kubelet from the runtime specifics.

In order to generate pod lifecycle events, PLEG needs to detect changes in container states. The
PLEG module periodically relisting all containers (even then stopped ones) and compare then with
their Kubelet's PodSpecs. The relist process takes longer when there are problems with the
underlying CRI or overloading of a Node with too many pods.

See more about the PLEG's mechanism in
the [Redhat's blogspot](https://developers.redhat.com/blog/2019/11/13/pod-lifecycle-event-generator-understanding-the-pleg-is-not-healthy-issue-in-kubernetes#)

</details>

<details>
<summary>References and Sources</summary>

1. [Kubelet CLI in Kubernetes official docs](https://kubernetes.io/docs/reference/command-line-tools-reference/kubelet/)
2. [PLEG mechanism explained in Redhat's blogspot](https://developers.redhat.com/blog/2019/11/13/pod-lifecycle-event-generator-understanding-the-pleg-is-not-healthy-issue-in-kubernetes#)

</details>

### Troubleshooting

Most cloud providers address this issue by limiting the Pods that can run in particular nodes in
their managed Kubernetes services. They often implement health checks into the underlying container
runtime. However, you may encounter this issue in high-end nodes which can run hundreds of
containers. If you have configured your cluster by yourself (let's say with `kubeadm`), you can
update the value of max Pods.
