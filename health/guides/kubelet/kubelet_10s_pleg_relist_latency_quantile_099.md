### Understand the alert

This alert is related to the Kubernetes Kubelet, which is the primary node agent responsible for ensuring containers run in a Pod. The alert specifically relates to the Pod Lifecycle Event Generator (PLEG) module, which is responsible for adjusting the container runtime state and maintaining the Pod's cache. When there is a significant increase in the relisting time for PLEG, you'll receive a `kubelet_10s_pleg_relist_latency_quantile_099` alert.

### Troubleshoot the alert

Follow the steps below to troubleshoot this alert:

1. Check the container runtime health status

   If you are using Docker as the container runtime, run the following command:

   ```
   sudo docker info
   ```

   Check for any reported errors or issues.

   If you are using a different container runtime like containerd or CRI-O, refer to the respective documentation for health check commands.

2. Check Kubelet logs for any errors.

   You can do this by running the following command:

   ```
   sudo journalctl -u kubelet -n 1000
   ```

   Look for any relevant error messages or warnings in the output.

3. Validate that the node is not overloaded with too many Pods.

   Run the following commands:

   ```
   kubectl get nodes
   kubectl describe node <node_name>
   ```

   Adjust the max number of Pods per node if needed, by editing the Kubelet configuration file `/etc/systemd/system/kubelet.service.d/10-kubeadm.conf`, adding the `--max-pods=<NUMBER>` flag, and restarting Kubelet:

   ```
   sudo systemctl daemon-reload
   sudo systemctl restart kubelet
   ```

4. Check for issues related to the underlying storage or network.

   Inspect the Node's storage and ensure there are no I/O limitations or bottlenecks causing the increased latency. Also, check for network-related issues that could affect the communication between the Kubelet and the container runtime.

5. Verify the performance and health of the Kubernetes API server.

   High workload on the API server could affect the Kubelet's ability to communicate and process Pod updates. Check the API server logs and metrics to find any performance bottlenecks or errors.

### Useful resources

1. [Kubelet CLI in Kubernetes official docs](https://kubernetes.io/docs/reference/command-line-tools-reference/kubelet/)
2. [PLEG mechanism explained in Redhat's blogspot](https://developers.redhat.com/blog/2019/11/13/pod-lifecycle-event-generator-understanding-the-pleg-is-not-healthy-issue-in-kubernetes#)