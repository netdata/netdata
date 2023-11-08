### Understand the alert

This alert monitors the time since the Consul Raft leader server was last able to contact its follower nodes. If the time since the last contact exceeds the warning or critical thresholds, the alert will be triggered. High values indicate a potential issue with the Consul Raft leader's connection to its follower nodes.

### Troubleshoot the alert

1. Check Consul logs

Inspect the logs of the Consul leader server and follower nodes for any errors or relevant information. You can find the logs in `/var/log/consul` by default. 

2. Verify Consul agent health

Ensure that the Consul agents running on the leader and follower nodes are healthy. Use the following command to check the overall health:

   ```
   consul members
   ```

3. Review networking connectivity

Check the network connectivity between the leader and follower nodes. Verify the nodes are reachable, and there are no firewalls or security groups blocking the necessary ports. Consul uses these ports by default:

   - Server RPC (8300)
   - Serf LAN (8301)
   - Serf WAN (8302)
   - HTTP API (8500)
   - DNS Interface (8600)

4. Monitor Consul server's resource usage

Ensure that the Consul server isn't facing any resource constraints, such as high CPU, memory, or disk usage. Use system monitoring tools like `top`, `vmstat`, or `iotop` to observe resource usage and address bottlenecks.

5. Verify the Consul server configuration

Examine the Consul server's configuration file (usually located at `/etc/consul/consul.hcl`) and ensure that there are no errors, inconsistencies, or misconfigurations with server addresses, datacenter names, or communication settings.

### Useful resources

1. [Consul Docs: Troubleshooting](https://developer.hashicorp.com/consul/tutorials/datacenter-operations/troubleshooting)
2. [Consul Docs: Agent Configuration](https://www.consul.io/docs/agent/options)
