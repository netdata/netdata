# mysql_galera_cluster_status

## Database | MySQL, MariaDB

This alert presents the status of the Galera node cluster component. If you receive this, it is an
indication the cluster has been split into several components due to network failure.

<details><summary>What is Quorum</summary>

> A majority (> 50%) of nodes. In the event of a network partition, only the cluster partition
> that retains a quorum (if any) will remain Primary by default.<sup>[1](https://galeracluster.com/library/documentation/glossary.html#:~:text=A%20majority%20(%3E%2050%25)%20of%20nodes.%20In%20the%20event%20of%20a%20network%20partition%2C%20only%20the%20cluster%20partition%20that%20retains%20a%20quorum%20(if%20any)%20will%20remain%20Primary%20by%20default.) </sup>
</details>

<details><summary>What is a Primary Component?</summary>

> In addition to single-node failures, the cluster may be split into several components due to
> network failure. In such a situation, only one of the components can continue to modify the
> database state to avoid history divergence. This component is called the Primary Component (PC)
> .<sup>[1](https://galeracluster.com/library/documentation/glossary.html#:~:text=from%20the%20database.-,Primary%20Component,For%20more%20information%20on%20the%20Primary%20Component%2C%20see%20Quorum%20Components.,-Quorum) </sup>
</details>

<details><summary>What is a Non-primary state Component?</summary>

> The clusters without the quorum enter the non-primary state and begin attempt to connect with the
> Primary Component.<sup>[2](https://galeracluster.com/library/documentation/weighted-quorum.html#:~:text=while%20those%20without%20quorum%20enter%20the%20non%2Dprimary%20state%20and%20begin%20attempt%20to%20connect%20with%20the%20Primary%20Component.) </sup>
</details>

The codes of the Galera node cluster component status can be:

| Code | Description              | Alert Status |
|:----:|:-------------------------|:------------:|
| `-1` | Unknown.                 |   Critical   |
| `0`  | Primary                  |    Clear     |
| `1`  | Non-primary/quorum lost  |   Critical   |
| `2`  | Disconnected             |   Critical   |

For further information on Primary and non-Primary components please have a look at the 
*References and Sources* section.

<details><summary>References and Sources</summary>

1. [Galera CLuster Glossary](
   https://galeracluster.com/library/documentation/glossary.html)
2. [Galera Cluster Documentation](
   https://galeracluster.com/library/documentation/weighted-quorum.html)

</details>