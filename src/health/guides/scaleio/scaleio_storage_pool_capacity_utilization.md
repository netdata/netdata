### Understand the alert

The `scaleio_storage_pool_capacity_utilization` alert is related to storage capacity in ScaleIO, a software-defined storage solution. If you receive this alert, it means that the storage pool capacity utilization is high, potentially leading to performance issues or running out of space.

### What does high storage pool capacity utilization mean?

High storage pool capacity utilization means that the allocated storage space in the ScaleIO storage pool is being used at a high percentage. Warning and critical alerts are triggered at 80-90% and 90-98% utilization, respectively. When the storage pool capacity utilization is high, it may impact the performance of the system and may prevent new data from being stored, as available space is limited.

### Troubleshoot the alert

1. **Verify the storage pool capacity utilization**

   Check the Netdata dashboard or use Netdata API to verify the storage pool capacity utilization. Take note of the storage pools with high utilization.

2. **Investigate storage usage**

   Inspect the storage usage in your environment, and determine which data or applications are consuming the most space. You can use tools like `du`, `df`, and `ncdu` to analyze disk usage.

3. **Delete or move unnecessary files**

   If you found any unnecessary files or backup copies occupying large amounts of space, consider deleting them or moving them to different storage devices to free up space in the storage pool.

4. **Optimize storage provisioning**

   Evaluate the storage provisioning for your applications, and ensure that appropriate storage space is allocated based on the actual needs. Adjust storage allocations if needed.

5. **Consider expanding the storage pool**

   If the high storage pool capacity utilization is expected based on your application and data storage needs, consider expanding the storage pool by adding new devices or increasing the allocated storage space on the existing devices in the pool.

6. **Monitor storage pool capacity utilization trends**

   Keep track of the storage pool capacity utilization trends and be proactive in addressing potential storage capacity issues in the future.

