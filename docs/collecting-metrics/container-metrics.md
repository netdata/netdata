# Collect Container Metrics with Netdata

Thanks to close integration with Linux cgroups and the virtual files it maintains under `/sys/fs/cgroup`, Netdata can
monitor health, status, and resource utilization of many different types of Linux containers.

Netdata uses the [cgroups.plugin](/src/collectors/cgroups.plugin/README.md) to poll `/sys/fs/cgroup` and convert the raw data into human-readable metrics and meaningful visualizations. Through cgroups, Netdata is compatible with **all Linux containers**, such as Docker, LXC, LXD, Libvirt, systemd-nspawn, and more. Read more about [Docker-specific monitoring](#collect-docker-metrics) below.

Netdata also has robust **Kubernetes monitoring** support thanks to a [Helmchart](/packaging/installer/methods/kubernetes.md) to automate deployment, collectors for k8s agent services, and robust [service discovery](https://github.com/netdata/agent-service-discovery/#service-discovery) to monitor the services running inside of pods in your k8s cluster. Read more about [Kubernetes monitoring](#collect-kubernetes-metrics) below.

A handful of additional collectors gather metrics from container-related services, such as [dockerd](/src/go/collectors/go.d.plugin/modules/docker/README.md) or [Docker Engine](/src/go/collectors/go.d.plugin/modules/docker_engine/README.md). You can find all
container collectors in our data collection integrations list under the [containers/VMs](/src/collectors/COLLECTORS.md#containers-and-vms) and [Kubernetes](/src/collectors/COLLECTORS.md#kubernetes) sections.

## Collect Docker metrics

Netdata has robust Docker monitoring thanks to the aforementioned [cgroups.plugin](/src/collectors/cgroups.plugin/README.md).

Netdata also collects metrics from applications running inside of Docker containers. For example, if you create a MySQL database container using `docker run --name some-mysql -e MYSQL_ROOT_PASSWORD=my-secret-pw -d mysql:tag`, it exposes metrics on port 3306. You can configure the [MySQL collector](/src/go/collectors/go.d.plugin/modules/mysql/README.md) to look at `127.0.0.0:3306` for MySQL metrics:

```yml
jobs:
  - name: local
    dsn: root:my-secret-pw@tcp(127.0.0.1:3306)/
```

Netdata then collects metrics from the container itself, but also dozens [MySQL-specific metrics](/src/go/collectors/go.d.plugin/modules/mysql/README.md#charts) as well.

### Collect metrics from applications running in Docker containers

You could use this technique to monitor an entire infrastructure of Docker containers. The same [enable and configure](/src/collectors/REFERENCE.md) procedures apply whether an application runs on the host system or inside a container. You may need to configure the target endpoint if it's not the application's default.

Netdata can even [run in a Docker container](/packaging/docker/README.md) itself, and then collect metrics about the host system, its own container with cgroups, and any applications you want to monitor.

See our [application metrics doc](/docs/collecting-metrics/application-metrics.md) for details about Netdata's application metrics collection capabilities.

## Collect Kubernetes metrics

Netdata provides a host of tools and collectors for monitoring the many layers of a Kubernetes cluster. These methods work together to help you troubleshoot performance or availability issues across your k8s infrastructure.

- A [Helm chart](https://github.com/netdata/helmchart), which bootstraps a Netdata Agent pod on every node in your cluster, plus an additional parent pod for storing metrics and managing alert notifications.
- A [service discovery plugin](https://github.com/netdata/agent-service-discovery), which discovers and creates configuration files for [compatible applications](https://github.com/netdata/helmchart#service-discovery-and-supported-services) and any endpoints covered by our [generic Prometheus collector](/src/go/collectors/go.d.plugin/modules/prometheus/README.md). With these configuration files, Netdata collects metrics from any compatible applications as they run _inside_ a pod. Service discovery happens without manual intervention as pods are created, destroyed, or moved between nodes.
- A [Kubelet collector](/src/go/collectors/go.d.plugin/modules/k8s_kubelet/README.md), which runs on each node in a k8s cluster to monitor the number of pods/containers, the volume of operations on each container, and more.
- A [kube-proxy collector](/src/go/collectors/go.d.plugin/modules/k8s_kubeproxy/README.md), which also runs on each node and monitors latency and the volume of HTTP requests to the proxy.
- A [cgroups collector](/src/collectors/cgroups.plugin/README.md), which collects CPU, memory, and bandwidth metrics for each container running on your k8s cluster.
