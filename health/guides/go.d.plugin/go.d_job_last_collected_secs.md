# go.d_job_last_collected_secs

**Netdata | go.d.plugin**

The Netdata Agent also monitors itself, so this is an alert about the Netdata go.d plugin. The Netdata Agent 
keeps track of the number of seconds since the last successful data collection for each data collection job. 
This alert indicates that a particular job has failed to collect metrics for several consecutive attempts.

You can see all the modules that are orchestrated by the go.d.plugin in
our [go.d.plugin documentation page](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin#available-modules)

### Troubleshooting section

<details>
<summary>Check the Netdata logs</summary>

You need to identify why the Agent cannot collect metrics for a specific job. Inspect the Agent logs for this
specific job.
  
Host machine: 
  
  ```
  root@netdata # tail -f /var/log/netdata/error.log | grep <job> OR <module_name>
  ```

Docker:

  ```
  root@netdata # docker logs <netdata_container> 2>&1 | grep <job> OR <module_name>
  ```
  
Kubernetes:
  1. Find the pod name of the node which produced the alert.
  
    ```
    kubectl -n <namespace> get pod -o wide -l app=netdata | grep <node_name>
    ```
  2. Inspect it's logs
  
    ```
    root@netdata # kubectl logs -n <namespace> <pod_name> | grep <job> OR <module_name>
    ```
  
</details>
