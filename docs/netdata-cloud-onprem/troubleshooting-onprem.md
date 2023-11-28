# Basic troubleshooting
We cannot predict how your particular installation of Netdata Cloud On-prem is going to work. It is a mixture of underlying infrastructure, the number of agents, and their topology.
You can always contact the Netdata team for recommendations!

#### Loading charts takes a long time or ends with an error
Charts service is trying to collect the data from all of the agents in question. If we are talking about the overview screen, all of the nodes in space are going to be queried (`All nodes` room). If it takes a long time, there are a few things that should be checked:
1. How many nodes are you querying directly?
  There is a big difference between having 100 nodes connected directly to the cloud compared to them being connected through a few parents. Netdata always prioritizes querying nodes through parents. This way, we can reduce some of the load by pushing the responsibility to query the data to the parent. The parent is then responsible for passing accumulated data from nodes connected to it to the cloud.
1. If you are missing data from endpoints all the time.
  Netdata Cloud always queries nodes themselves for the metrics. The cloud only holds information about metadata, such as information about what charts can be pulled from any node, but not the data points themselves for any metric. This means that if a node is throttled by the network connection or under high resource pressure, the information exchange between the agent and cloud through the MQTT broker might take a long time. In addition to checking resource usage and networking, we advise using a parent node for such endpoints. Parents can hold the data from nodes that are connected to the cloud through them, eliminating the need to query those endpoints.
1. Errors on the cloud when trying to load charts.
  If the entire data query is crashing and no data is displayed on the UI, it could indicate problems with the `cloud-charts-service`. The query you are performing might simply exceed the CPU and/or memory limits set on the deployment. We advise increasing those resources.
It takes a long time to load anything on the Cloud UI
When experiencing sluggishness and slow responsiveness, the following factors should be checked regarding the Postgres database:
  1. CPU: Monitor the CPU usage to ensure it is not reaching its maximum capacity. High and sustained CPU usage can lead to sluggish performance.
  1. Memory: Check if the database server has sufficient memory allocated. Inadequate memory could cause excessive disk I/O and slow down the database.
  1. Disk Queue / IOPS: Analyze the disk queue length and disk I/O operations per second (IOPS). A high disk queue length or limited IOPS can indicate a bottleneck and negatively impact database performance.
By examining these factors and ensuring that CPU, memory, and disk IOPS are within acceptable ranges, you can mitigate potential performance issues with the Postgres database.

#### Nodes are not updated quickly on the Cloud UI
If you're experiencing delays with information exchange between the Cloud UI and the Agent, and you've already checked the networking and resource usage on the agent side, the problem may be related to Apache Pulsar or the database. Slow alerts on node alerts or slow updates on node status (online/offline) could indicate issues with message processing or database performance. You may want to investigate the performance of Apache Pulsar, ensure it is properly configured, and consider scaling or optimizing the database to handle the volume of data being processed or written to it.
