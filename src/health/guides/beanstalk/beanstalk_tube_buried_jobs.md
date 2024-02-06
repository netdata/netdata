### Understand the alert

This alert monitors the number of buried jobs in each beanstalkd tube. If you receive this alert, it means that there are jobs that have been buried, and you need to investigate the cause. The warning threshold is set at more than zero buried jobs, and the critical threshold is set at more than ten buried jobs.

### What are buried jobs?

In Beanstalkd, buried jobs are jobs that were moved to the buried state deliberately or jobs that have failed repeatedly. They're kept in a separate queue and will not be processed by workers until they're explicitly handled or deleted.

### Troubleshoot the alert

1. Check the Beanstalkd logs for any errors or pertinent information related to the buried jobs. You can find the logs in the `/var/log/beanstalkd.log` file (the default log file location) or any other custom location defined for Beanstalkd.

2. Use the `beanstalk-console` or a similar tool to inspect the buried jobs to determine their causes. You can download `beanstalk-console` [here](https://github.com/ptrofimov/beanstalk_console).

3. Review the applications or workers that are interacting with the affected tubes to find any possible issues or bugs.

4. If the buried jobs are blocking the processing of other jobs, consider moving them to another tube with higher priority or increase the number of workers processing the tube.

5. If the buried jobs are safe to delete or requeue, do so to clear the count and alleviate the alert. You can use the following commands to kick or delete jobs using the `beanstalk-cli`:
   ```
   beanstalk-cli kick-job [<job-id>]
   beanstalk-cli delete-job [<job-id>]
   ```

6. If none of the steps above help mitigate the issue, consider contacting the sysadmin or developers of the application using Beanstalkd.

### Useful resources

1. [Beanstalkd Protocol](https://raw.githubusercontent.com/beanstalkd/beanstalkd/master/doc/protocol.txt)
2. [Beanstalk_console - a web-based beanstalk queue server console](https://github.com/ptrofimov/beanstalk_console)
