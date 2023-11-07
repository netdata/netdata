# cockroachdb_unavailable_ranges

## Database | CockroachDB

This alert presents the number of unavailable ranges. If you receive this, it indicates that there
are ranges with fewer live replicas than needed for quorum.

This alert is raised in a warning state when unavailable ranges start to exist.

<details><summary>What are unavailable ranges?</summary>
   
> Unavailable ranges: If a majority of a range's replicas are on nodes that are unavailable,
> then the entire range is unavailable and will be unable to process queries.
>
> CockroachDB uses consensus replication and requires a quorum of the replicas to
> be available in order to allow both writes and reads to the range. The number of failures
> that can be tolerated is equal to (Replication factor - 1)/2. Thus, CockroachDB requires (n-1)
> /2 nodes to achieve quorum. For example, with 3x replication, one failure can be tolerated;
> with 5x replication, two failures, and so on.<sup>[1](
> https://www.cockroachlabs.com/docs/stable/cluster-setup-troubleshooting.html#db-console-shows-under-replicated-unavailable-ranges) </sup>

</details>

<details><summary>References and Sources</summary>

1. [CockroachDB docs](
   https://www.cockroachlabs.com/docs/stable/cluster-setup-troubleshooting.html#db-console-shows-under-replicated-unavailable-ranges)

</details>

### Troubleshooting Section

<details><summary>Identify unavailable ranges</summary>

Check out the [CockroachDB documentation](
https://www.cockroachlabs.com/docs/stable/cluster-setup-troubleshooting.html#db-console-shows-under-replicated-unavailable-ranges) for troubleshooting advice.


</details>
