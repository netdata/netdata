# vernemq_netsplits

**Messaging | VerneMQ**

A netsplit (also known as split-brain) is mostly the result of a failure of one or more network devices resulting in a cluster
where nodes can no longer reach each other.

The VerneMQ documentation explains how VerneMQ [deals with netsplits](https://docs.vernemq.com/v/master/vernemq-clustering/netsplits).

The Netdata Agent monitors the number of detected netsplits within the last minute. This alert indicates a split-brain situation.

### Troubleshooting section

<details>
<summary>Check connectivity between nodes</summary>

You must ensure that the connectivity between your cluster nodes is valid. As soon as the partition
is healed, and connectivity reestablished, the VerneMQ nodes replicate the latest changes made to
the subscription data. This includes all the changes 'accidentally' made during the window of
uncertainty. VerneMQ uses dotted version vectors to ensure that convergence regarding subscription
data and retained messages is eventually reached.

</details>


