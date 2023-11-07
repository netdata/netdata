# ceph_cluster_space_usage

**Storage | Ceph**

Ceph is an open-source software-defined storage platform that implements object storage on a single
distributed computer cluster and provides 3-in-1 interfaces for object-, block- and file-level
storage. Ceph aims primarily for completely distributed operation without a single point of failure,
scalability to the exabyte level, and to be freely available.

The Netdata Agent calculates the percentage of used cluster disk space. Your cluster is in high disk
space utilization.

This alert is triggered in warning state when the percentage of used cluster disk space is between
85-90% and in critical state when it is between 90-98%.

### Troubleshooting section

Data is priceless. Before you perform any action, make sure that you have taken any necessary backup
steps. Netdata is not liable for any loss or corruption of any data, database, or software.


<details>
<summary>Examine your cluster status </summary>

1. In the master node, examine the disk space details of the cluster

    ```
    root@netdata # ceph df detail
   
    ```

2. Check for unused pools and delete them or and consider adding a node to your cluster.

</details>