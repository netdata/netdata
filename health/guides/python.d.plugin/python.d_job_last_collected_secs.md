# python.d_job_last_collected_secs

**Netdata | python.d.plugin**

The Netdata Agent also monitors itself, so this is an alert about the Netdata Python plugin. The
Netdata Agent monitors the number of seconds since the last successful data collection for each of
python plugin modules. This alert indicates that a specific module cannot reach the component it
monitors to collect metrics from it.

You can see all the modules that are orchestrated by the python.d.plugin
in [our GitHub repo](https://github.com/netdata/netdata/tree/master/collectors/python.d.plugin)

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
