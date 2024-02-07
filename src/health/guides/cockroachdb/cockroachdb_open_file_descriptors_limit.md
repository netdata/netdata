### Understand the alert

This alert indicates that the usage of file descriptors in your CockroachDB is reaching a high percentage against the soft-limit. High file descriptor utilization can cause issues, such as failures to open new files or establish network connections.

### Troubleshoot the alert

1. Check the current file descriptor limit and usage for CockroachDB:

   Use the `lsof` command to display information about all open file descriptors associated with the process running CockroachDB:

   ```
   lsof -p <PID>
   ```

   Replace `<PID>` with the process ID of CockroachDB.

   To display only the total number of open file descriptors, you can use this command:

   ```
   lsof -p <PID> | wc -l
   ```

2. Monitor file descriptor usage:

   Regularly monitoring file descriptor usage can help you identify patterns and trends, making it easier to determine if adjustments are needed. You can use tools like `lsof` or `sar` to monitor file descriptor usage on your system.

3. Adjust the file descriptors limit for the process:

   You can raise the soft-limit for the CockroachDB process by modifying the `ulimit` configuration:

   ```
   ulimit -n <new_limit>
   ```

   Replace `<new_limit>` with the desired value, which must be less than or equal to the system-wide hard limit.

   Note that changes made using `ulimit` only apply to the current shell session. To make the changes persistent, you should add the `ulimit` command to the CockroachDB service startup script or modify the system-wide limits in `/etc/security/limits.conf`.

4. Adjust the system-wide file descriptors limit:

   If necessary, you can also adjust the system-wide limits for file descriptors in `/etc/security/limits.conf`. Edit this file as a root user, and add or modify the following lines:

   ```
   * soft nofile <new_soft_limit>
   * hard nofile <new_hard_limit>
   ```

   Replace `<new_soft_limit>` and `<new_hard_limit>` with the desired values. You must restart the system or CockroachDB for the changes to take effect.

5. Optimize CockroachDB configuration:

   Review the CockroachDB configuration and ensure that it's optimized for your workload. If appropriate, adjust settings such as cache size, query optimization, and memory usage to reduce the number of file descriptors needed.

### Useful resources

1. [CockroachDB recommended production settings](https://www.cockroachlabs.com/docs/v21.2/recommended-production-settings#file-descriptors-limit)
2. [Increasing file descriptor limits on Linux](https://www.tecmint.com/increase-set-open-file-limits-in-linux/)
