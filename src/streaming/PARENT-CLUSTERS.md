# Notes on Netdata Active-Active Parent Clusters

#### **Streaming Connection Overview**

Each Netdata child node specifies its parent nodes through the
`[stream].destination` configuration in `stream.conf`. While a child can list
multiple parents, it will connect to only one at a time. If the connection to
the first parent fails, the child will try the next parent in the list,
continuing this process until a successful connection is established. If no
connection can be made, the child will retry the list in order.

Once a Netdata parent receives data from its child nodes, it can also act as a
child to another parent (or "grandparent") to propagate the data further up the
hierarchy.

#### **Active-Active Parent Clusters Overview**

Active-active parent clusters involve circular data propagation among parent
nodes. For example, parent A streams its data to parent B (its grandparent),
while parent B streams back to parent A, creating redundancy. This configuration
ensures that each parent node has the same data, allowing child nodes to connect
to any available parent.

This setup can be expanded to more than two parents by configuring all parents
as grandparents of each other.

---

### **Data Replication**

When a child node connects to a parent, it enters a negotiation phase to
announce the metrics it will stream, including their retention period. The
parent checks its database for missing data. If data gaps exist, the parent
requests replication of the missing metrics from the child before transitioning
to streaming fresh data.

Replication occurs at the instance (metric group) level, meaning some metrics
may replicate historical data while others stream in real time. Only high-
resolution (`tier0`) data is replicated since higher-tier data can be derived
from `tier0`. Therefore, maintaining sufficient `tier0` retention on the child
is crucial to prevent gaps in the parent’s database.

---

### **Challenges in Active-Active Clusters**

#### **Adding a New Parent**

Introducing a new parent to an active-active cluster involves two major
challenges:

1. **Replicating Existing Data**  
   Since Netdata replication only propagates currently collected metrics,
   archived data such as metrics from stopped containers or disconnected devices
   will not be replicated. To ensure the new parent has complete historical
   data:
    - Copy the existing database from another parent (`/var/cache/netdata`) using
      tools like `rsync` for dbengine files (safe for hot-copy) and `sqlite3` for
      SQLite databases.
    - Perform multiple copies and start the new parent promptly to minimize the
      data gap.

2. **Preventing Premature Connections**  
   Child nodes should not connect to the new parent until it has completed data
   replication. Premature connections could lead to data gaps, as the child may
   lack the necessary historical data.

   In Netdata v2.1+, a balancing feature allows children to query parent
   retention and prioritize connections to parents with the most recent data.
   However, children will still connect to the first available parent,
   potentially introducing gaps to the new parent's database.  
   **Solution:** Keep the new parent isolated from children until its
   replication process is complete. Only then should children be configured to
   include the new parent.

---

### **Resource Management in Clusters**

Resource usage on parent nodes depends on three key factors:

1. **Ingestion Rate**  
   All parent nodes ingest all data of all children (not just their own). The
   resource load is the same across all parents.

2. **Machine Learning**  
   Machine learning is CPU-intensive and affects memory usage.
    - **Before Netdata 2.1:** Every node in a cluster independently trained
      machine learning models for all children, increasing resource consumption
      exponentially.
    - **Netdata 2.1+:** The first node (child or parent) to train ML models
      propagates the trained data to other nodes, significantly reducing resource
      requirements. This allows flexibility: machine learning can either run at
      the edge (child nodes) or on the first parent receiving the data.

3. **Re-Streaming Rate**  
   Propagating data to other parents consumes CPU and bandwidth for formatting,
   compressing, and transmitting data. Each parent except the last grandparent
   in the chain contributes to this workload.

---

### **Parent Balancing**

Netdata v2.1 introduces a balancing algorithm for child nodes to optimize parent
connections. This feature is designed to ensure that child nodes connect to the
most suitable parent, balancing the load across the cluster and reducing the
likelihood of data gaps or resource bottlenecks.

#### **Initial Balancing**

Before establishing a connection, each child node queries its candidate parents
to retrieve their retention details. Based on this information, the child
evaluates the parents and prioritizes those with the most recent data.

- **Retention Difference Threshold**:  
  Parents are considered equivalent if their retention times differ by less than
  two minutes. In such cases, the child selects a parent randomly to avoid
  overloading a single node. This randomness ensures an even distribution of
  connections when all candidate parents are equally suitable.

- **Disconnection Handling**:  
  To prevent overloading a parent during network disruptions, children
  temporarily block a recently disconnected parent for a randomized duration
  before attempting to reconnect. This cooldown period reduces the risk of
  repeated disconnections and ensures smoother reconnections.

#### **Re-balancing After Cluster Changes**

Currently, the only way to re-balance the cluster (i.e. to break the existing
connections so that the children nodes will connect to both parents), is to
restart the children.

---

### **Summary**

Active-active parent clusters in Netdata provide robust data redundancy and
flexibility. Properly configuring replication, balancing resources, and managing
parent-child connections ensures optimal performance and data integrity. With
improvements in v2.1, including machine learning propagation and connection
balancing, Netdata clusters are more efficient and scalable, catering to complex
and dynamic environments.
