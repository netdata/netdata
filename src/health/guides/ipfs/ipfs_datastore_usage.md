### Understand the alert

This alert is related to the InterPlanetary File System (IPFS) distributed file system. It calculates the percentage of used IPFS datastore space. When you receive this alert, it means that your IPFS storage repository space is highly utilized.

### What does high datastore usage mean?

High datastore usage means your IPFS storage is close to its capacity. This can affect the system's performance and stability. It is essential to keep an eye on IPFS storage usage to ensure smooth functioning and avoid running out of storage.

### Troubleshoot the alert

1. Check IPFS datastore usage

   To check the current IPFS datastore storage utilization, use the `ipfs repo stat` command:
   
   ```
   ipfs repo stat
   ```

2. Identify large files and folders within the datastore

   To find the largest files and folders within your IPFS datastore, use the following command:
   
   ```
   ipfs pin ls --type=recursive | xargs -n1 -I {} echo -n "{} " && ipfs object stat {} | head -n1 | awk '{print $2}'
   ```

3. Clean up IPFS datastore

   You can clean up and remove files that are no longer needed from your datastore using `ipfs pin rm` and `ipfs repo gc` commands. Be cautious while removing data to avoid losing any essential files.

   For example:

   ```
   ipfs pin rm <CID>
   ipfs repo gc
   ```

4. Consider increasing the size of your datastore

   If your datastore is continuously getting filled, you might need to increase its capacity to ensure smooth operation. This can be done by adjusting the `Datastore.StorageMax` configuration setting in the `config` file, which is typically located in the `.ipfs` folder.

   ```
   ipfs config Datastore.StorageMax <new size>
   ```

5. Monitor datastore usage over time

   Regularly monitor your IPFS datastore usage using `ipfs repo stat` command to stay informed about its storage utilization and plan for any necessary adjustments.

### Useful resources

1. [IPFS Documentation](https://docs.ipfs.io/)
2. [IPFS resize datastore](https://github.com/ipfs/go-ipfs/blob/master/docs/config.md#datastorestoragemax)
