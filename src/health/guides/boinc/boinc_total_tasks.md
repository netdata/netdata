### Understand the alert

This alert monitors the average number of total tasks for the BOINC system over the last 10 minutes. If you receive this alert, it means that there is a deviation in the number of total tasks for your BOINC system, which may indicate an issue with the projects, the client, or even the tasks themselves.

### Troubleshoot the alert

#### Verify the project status

1. Verify that the projects you contribute to are not suspended. You can check if the project has queued tasks to be done on the [BOINC projects page](https://boinc.berkeley.edu/projects.php).

2. Access your BOINC Manager, go to the _Projects_ tab, and check if the projects you contribute to are in the correct state (Active or Running). If a project is suspended, you can select it and click _Resume_ to reactivate it.

#### Investigate task issues

1. Access your BOINC Manager and go to the _Tasks_ tab to check the status of the current tasks. Look for any _Failed_, _Error_, or _Postponed_ tasks.

2. If there are failed tasks, try to reset them by selecting the task, right-clicking on it, and choosing _Update_ or _Reset_. Be aware that resetting a task will discard any progress made on it.

#### Restart the BOINC client

1. For most Linux distributions:

   ```
   sudo /etc/init.d/boinc-client restart
   ```

#### Check system resources

BOINC tasks may fail or slow down if there is not enough system resources (CPU, RAM, or Disk Space) available. Monitor your system performance using tools like `top`, `free`, and `df`, and make adjustments if necessary to ensure that BOINC has enough resources to complete tasks.

