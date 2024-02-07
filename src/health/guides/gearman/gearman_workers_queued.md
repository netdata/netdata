### Understand the alert

This alert is related to the Gearman application framework. If you receive this alert, it means that the average number of queued jobs in the last 10 minutes is significantly high, indicating that more workers may be needed to maintain an efficient workflow.

### What is Gearman?

Gearman is an open-source, distributed job scheduling framework that allows applications to distribute processing tasks among multiple worker machines. It is useful to parallelize tasks and manage workloads between different systems.

### Troubleshoot the alert

1. Check the status of Gearman with the following command:

   ```
   gearadmin --status
   ```

2. Analyze the output and identify queues with a high number of jobs:

   Example output:

   ```
   queue1    50000    10    0
   queue2    65000    20    0
   ```

   In this example, `queue1` and `queue2` have a high number of queued jobs (50,000 and 65,000), with 10 and 20 workers working on them respectively.

3. Increase the number of workers:

   To increase the number of workers, you may need to start additional worker instances or adjust the configurable number of workers in your Gearman deployment. For instance, if you use a script to start workers, you can update this script and start more instances.

4. Monitor the Gearman metrics:

   Continue to monitor the metrics for some time to ensure that the additional workers are effectively reducing the number of queued jobs.

5. If necessary, further optimize the Gearman deployment:

   If the problem persists, you may need to analyze the queues in further detail, such as looking into possible bottlenecks, inefficient operations, or other performance-related factors.

### Useful resources

1. [Monitoring Gearman with Netdata](https://www.netdata.cloud/gearman-monitoring/)
2. [Gearman Documentation](http://gearman.org/documentation/)
