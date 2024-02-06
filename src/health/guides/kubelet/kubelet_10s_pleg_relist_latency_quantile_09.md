### Understand the alert

This alert indicates that the average relisting latency of the Pod Lifecycle Event Generator (PLEG) in Kubelet over the last 10 seconds compared to the last minute (quantile 0.9) has increased significantly. This can cause the node to become unavailable (NotReady) due to a "PLEG is not healthy" event.

### Troubleshoot the alert

1. Check for high node resource usage

   First, ensure that the node does not have an overly high number of Pods. High resource usage could increase the PLEG relist latency, leading to poor Kubelet performance. You can check the current number of running Pods on a node using the following command:

   ```
   kubectl get pods --all-namespaces -o wide | grep <node-name>
   ```

2. Check Kubelet logs for errors

   Inspect the Kubelet logs for any errors that might be causing the increased PLEG relist latency. You can check the Kubelet logs using the following command:

   ```
   sudo journalctl -u kubelet
   ```
   
   Look for any errors associated with PLEG or the container runtime, such as Docker or containerd.

3. Check container runtime health

   If you find any issues in the Kubelet logs related to the container runtime, investigate the health of the container runtime, such as Docker or containerd, and its logs to identify any issues:

   - For Docker, you can check its health using:

     ```
     sudo docker info
     sudo journalctl -u docker
     ```

   - For containerd, you can check its health using:

     ```
     sudo ctr version
     sudo journalctl -u containerd
     ```
   
4. Adjust the maximum number of Pods per node

   If you have configured your cluster manually (e.g., with `kubeadm`), you can update the value of max Pods in the Kubelet configuration file. The default file location is `/var/lib/kubelet/config.yaml`. Change the `maxPods` value according to your requirements and restart the Kubelet service:

   ```
   sudo systemctl restart kubelet
   ```

5. Monitor the PLEG relist latency

   After making any necessary changes, continue monitoring the PLEG relist latency to ensure the issue has been resolved.

### Useful resources

1. [Kubelet CLI in Kubernetes official docs](https://kubernetes.io/docs/reference/command-line-tools-reference/kubelet/)
2. [PLEG mechanism explained in Redhat's blogspot](https://developers.redhat.com/blog/2019/11/13/pod-lifecycle-event-generator-understanding-the-pleg-is-not-healthy-issue-in-kubernetes#)