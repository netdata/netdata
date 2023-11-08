### Understand the alert

This alert calculates the average number of packets received by a specific network interface (denoted as `${label:device}` in the alert) on a Kubernetes cluster node over the last minute. If you receive this alert, it indicates that there is a significant amount of network traffic received by the node.

### What does high received packets rate mean?

A high received packets rate means that the network interface on the Kubernetes cluster node is processing a large number of incoming network packets. This can be due to increased legitimate traffic to the services running on the cluster or may indicate a potential network issue or Distributed Denial of Service (DDoS) attack.

### Troubleshoot the alert

1. Verify the current network traffic on the Kubernetes node:

   You can use the `nethogs` tool to analyze the network traffic on the Kubernetes node. If the tool is not installed, you can install it with:

   ```
   sudo apt install nethogs  # Ubuntu/Debian
   sudo yum install nethogs  # CentOS/RHEL
   ```

   Run `nethogs` to check the network traffic:

   ```
   sudo nethogs
   ```

2. Check the services running on the Kubernetes cluster:

   Use the command `kubectl get pods --all-namespaces` to list all the pods running on the cluster. Inspect the output and identify any services that might be consuming a high amount of network traffic. 

3. Inspect logs for any anomalies:

   Check the application and Kubernetes logs for any unusual activity, errors, or repeated access attempts that may indicate a network issue or potential attack.

4. Close unnecessary processes or services:

   Based on your analysis, if you find any unnecessary processes or services consuming a high amount of network traffic, consider terminating or scaling them down.

5. Check for DDoS attacks:

   If you suspect a DDoS attack, consider implementing traffic filtering, rate limiting, or using a DDoS protection service to mitigate the attack.

6. Monitor network traffic:

   Continue monitoring the network traffic on the Kubernetes node to ensure that the received packets rate returns to normal levels.

### Useful resources

1. [Kubernetes Networking](https://kubernetes.io/docs/concepts/cluster-administration/networking/)
2. [How to Monitor and Identify Issues with Kubernetes Networking](https://www.stackrox.com/post/2017/03/how-to-monitor-and-identify-issues-with-kubernetes-networking/)
