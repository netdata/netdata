# mysql_connections

## Database | MySQL, MariaDB

This alert presents the percentage of used client connections.  
Receiving this alert means that there is a high client connection utilization 
compared to the limit.

This alert is raised to warning when the percentage exceeds 70%.  
If the metric exceeds 90%, then the alert is escalated to critical.

<details><summary>References and Sources</summary>

1. [MySQL max connections](https://ubiq.co/database-blog/how-to-increase-max-connections-in-mysql/)

</details>

### Troubleshooting Section

<details><summary>Increase the Connection Limit</summary>

To increase the connection limit, log into MySQL form the terminal and use the following code:  
`show variables like "max_connections";`  
to see the current limit.

Using:  
`set global max_connections = "LIMIT";`  
Where "LIMIT" is the new limit you will choose, you can alter the limit without restarting the
server.

To increase the limit permanently, locate the `my.cnf` file (typically under `/etc` but depends on
installation) and append `max_connections = 200` under the `mysqld` section.

You can read more in our References and Sources section.

</details>
