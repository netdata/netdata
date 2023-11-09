### Understand the alert

This alert presents the number of slow queries in the last 10 seconds. If you receive this, it indicates a high number of slow queries.

The metric is raised in a warning state when the value is larger than 10. If the number of slow queries in the last 10 seconds exceeds 20, then the alert is raised in critical state.

Queries are defined as "slow", if they have taken more than `long_query_time` seconds, a predefined variable. Also, the value is measured in real time, not CPU time.

### Troubleshoot the alert

- Determine which queries are the problem and try to optimise them

To identify the slow queries, you can enable the slow-query log of MySQL:  

1. Locate the `my.cnf` file 
2. Enable the slow-query log by setting the `slow_query_log variable` to `On`.
3. Enter a path where the log files should be stored in the `slow_query_log_file` variable.

After you know which queries are the ones taking longer than preferred, you can use the `EXPLAIN` keyword to overview how many rows are accessed, what operations are being done etc.

After you've found the cause for the slow queries, you can start optimizing your queries. Consider to use an index and think about how you can change the way you `JOIN` tables. Both of these methods aid to reduce the amount of data that is being accessed without it really being needed.

### Useful resources
[SQL Query Optimisation](https://opensource.com/article/17/5/speed-your-mysql-queries-300-times)

