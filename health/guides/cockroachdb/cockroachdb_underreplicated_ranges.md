# cockroachdb_underreplicated_ranges

## Database | CockroachDB

This alert presents the number of under-replicated ranges.

This alert is raised in a warning state when under-replicated ranges start to exist.

> Under-replicated ranges: When a cluster is first initialized, the few default starting ranges
> will only have a single replica, but as soon as other nodes are available, they will
> replicate to them until they've reached their desired replication factor. If a range does not
> have enough replicas, the range is said to be "under-replicated".
>
> CockroachDB uses consensus replication and requires a quorum of the replicas to
> be available in order to allow both writes and reads to the range. The number of failures
> that can be tolerated is equal to (Replication factor - 1)/2. Thus, CockroachDB requires (n-1)
> /2 nodes to achieve quorum. For example, with 3x replication, one failure can be tolerated;
> with 5x replication, two failures, and so on.<sup>[1](
> https://www.cockroachlabs.com/docs/stable/cluster-setup-troubleshooting.html#db-console-shows-under-replicated-unavailable-ranges) </sup>

<details><summary>References and Sources</summary>

1. [CockroachDB documentation](
   https://www.cockroachlabs.com/docs/stable/cluster-setup-troubleshooting.html#db-console-shows-under-replicated-unavailable-ranges)

</details>

### Troubleshooting Section

<details><summary>Identify under-replicated ranges</summary>

Check out the [CockroachDB documentation](
https://www.cockroachlabs.com/docs/stable/cluster-setup-troubleshooting.html#db-console-shows-under-replicated-unavailable-ranges)
for troubleshooting advice.
</details>
