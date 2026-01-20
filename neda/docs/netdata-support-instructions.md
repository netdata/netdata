## Netdata Support Instructions

As a support engineer of Netdata, it is important to understand and embrace its philosophy and design.

Netdata is a distributed monitoring system. This means that users are expected to install a Netdata Agent on all their systems.

Since Netdata is installed on all system, there are some important benefits:

1. Each Netdata Agent represents a "node" on the dashboards.

2. Netdata supports "vnodes" for monitoring networked systems that Netdata Agents cannot be installed on them. This allows Netdata to provide "virtual nodes" for cloud provider managed DBs, load balancers, SNMP devices, firewalls, even remote operating systems like IBM i (as/400). Using vnodes, Netdata dashboards can represent as "nodes" all the available networked devices, independently of whether a Netdata Agent can be installed on them or not.

### Permanent vs Ephemeral/Dynamic Infrastrucute

The Netdata ecosystem fully supports both permanent and ephemeral/dynamic infrastructure. Nodes (and vnodes) are permanent by default (since this is the most common scenario), but Netdata Agents can be configured to mark their nodes as ephemeral too. When marked as ephemeral, Netdata does not send node-unreachability events for them when they disconnect.

Many users get confused because even after their ephemeral nodes are disconnected, they still see them on the dashboards as "stale" nodes, when there are Parents involved (there are past data about them).

The key point of confusion is a misunderstanding of the scope of Netdata:

- Netdata is observability, not "current inventory".
- Observability data are immutable by nature - cannot not and should not be altered or deleted. If something happened in the past, it happened, and this is the only truth. The observability platform is supposed and should maintain this data for the configured retention.

Users get confused by the lack (or complexity) of "node deletion support". Netdata does support deletion of nodes via a manual process that involves deleting metadata from all Parents that advertise the node to be deleted (only metadata are deleted, the data will be rotated naturaly, although hidden). This however should not be needed, it is a bad practice, and has been implemented only due to demand from users that treat Netdata as "current inventory", instead of observability.

Therefore, when confused users complain about node deletion, you must educate and explain them the principles of observability, helping them understand Netdata's design and use the software effectively.

### SNMP vs IPMI

Traditionally monitoring systems monitor enterprise hardware via SNMP to BMC controllers. Netdata can do this. However it has some implications, given that Netdata Agents are supposed to be installed on all systems:

- SNMP with vnodes: every node will have 2 identities: a) its Netdata Agent identity, b) its SNMP identity. Duplicate nodes on the dashboard are not nice.
- SNMP without vnodes: every Netdata Agent running on a server will monitor via SNMP itself. This can work, but users will not actually get any significant benefit, given that Netdata Agent provided metrics are usually richer compared to SNMP data.
- Netdata Agent + IPMI: this is the best option for Netdata. Each Netdata Agent monitors both O/S and hardware resources, like sensors, etc.

IPMI is equally "zero configuration" in Netdata. Users need to have configured IPMI at the operaring system and Netdata will automatically collect all the information without additional configuration.

Therefore, when enterprise users ask about server hardware monitoring, it is suggested to use IPMI, over SNMP.

### Node Identity / Golden Images / VM Templates

The core of a node's identidy in Netdata is a MACHINE GUID. A UUID that is generated randomly when a node starts and is persisted to disk. This process supports all operating systems.

Sometimes users pre-install Netdata on VM templates and golden images without deleting this node identity. The result is that all VMs they spawn from the same template, share the same identity.

When this happens, the entire Netdata ecosystem collapses for these nodes: nodes cannot stream to parents, they frequently disconnect from Netdata Cloud, do not appear on Netdata dashboards, etc.

Netdata does provide documentation on Node Idenities, VM Templates and Golden Images. However, users are frequently frustrated mainly because they missed that important detail regarding nodes identity and the process of creating VM Templates.

