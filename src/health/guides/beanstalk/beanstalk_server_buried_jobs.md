### Understand the alert

This alert is related to the `Beanstalk` message queue system and is triggered when there are buried jobs in the queue across all tubes. A buried job is one that has encountered an issue during processing by the consumer, so it remains in the queue waiting for manual action. This alert is raised in a warning state if there are more than 0 buried jobs and in a critical state if there are more than 10.

### What are buried jobs?

Buried jobs are tasks that have faced an error or issue during processing by the consumer, and as a result, have been `buried`. This means these jobs remain in the queue, awaiting manual intervention for them to be processed again. The presence of buried jobs does not affect the processing of new jobs in the queue.

### Troubleshoot the alert

1. Identify the buried jobs: Use the `beanstalk-tool` to inspect the Beanstalk server and list the buried jobs in the tubes. If you don't have `beanstalk-tool`, install it using pip:

   ```
   pip install beanstalkc
   beanstalk-tool <beanstalk server host>:<beanstalk server port> stats_tube <tube_name>
   ```

2. Examine the buried jobs: To investigate the cause of the buried jobs, find related logs, either from the Beanstalk server or from the consumer application. Analyzing the logs can lead to the root cause of the problem.

3. Fix the issue: Once you identify the cause, resolve the issue in either the consumer application, or if necessary, in the Beanstalk server configuration.

4. Kick the buried jobs: After resolving the issue, you need to manually kick the buried jobs back into the queue for processing. Use the following command with `beanstalk-tool`:

   ```
   beanstalk-tool <beanstalk server host>:<beanstalk server port> kick <number_of_jobs_to_kick> --tube=<tube_name>
   ```

5. Monitor the queue: After kicking the buried jobs, monitor the queue and ensure that the jobs are processed without encountering more errors.

### Useful resources

1. [Beanstalk Documentation](https://beanstalkd.github.io/)
