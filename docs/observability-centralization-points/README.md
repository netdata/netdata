# **Observability Centralization Points**  

Netdata allows you to set up multiple **Observability Centralization Points** to aggregate metrics, logs, and metadata across your infrastructure.  

## **Why Use Centralization Points?**  

- **Ephemeral Systems**:  
  - Ideal for **Kubernetes nodes or temporary VMs** that frequently go offline.  
  - Ensures metrics and logs remain available for analysis and troubleshooting.  

- **Limited Resources**:  
  - Offloads observability tasks from systems with **low disk space, CPU, RAM, or I/O bandwidth**.  
  - Keeps production systems running efficiently without performance trade-offs.  

- **Multi-Node Dashboards Without Netdata Cloud**:  
  - Aggregates data from multiple nodes for **centralized dashboards**, similar to Netdata Cloud.  

- **Restricted Netdata Cloud Access**:  
  - Acts as a **bridge** when monitored systems can’t connect to Netdata Cloud due to **firewall restrictions**.  

## **How Multiple Centralization Points Work**  

- **With Netdata Cloud**:  
  - Queries all centralization points in parallel for a unified view of the infrastructure.  
- **Without Netdata Cloud**:  
  - Parent nodes consolidate data from connected systems, providing a local view of metrics and logs.