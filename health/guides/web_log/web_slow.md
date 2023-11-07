# web_slow

**Web Server | Web log**

When a client sends a request to your web server, there are many independent phases for this request
that can introduce delay. Some of them may be:

- DNS lookup
- Establish a TCP connection
- Perform a TLS handshake
- The server to process the request
- Transfer the actual content

The Netdata Agent calculates the average HTTP response time over the last minute for your web server
(NGINX, Apache). You receive this alert when your web server's average response time has increased. 
The alert is raised in warning when the average HTTP response time is twice as much. When the average HTTP response time is four times as much, you receive a critical alert.

### Troubleshooting section 

The causes of a slow request response may vary. Some options you can explore are:

<details>
<summary>Your web server utilization is high </summary>

This problem could be addressed on many levels:
- Check if your host machine can handle the traffic: Check the CPU, memory and traffic utilization. 
  If this is not an issue:
- Consider raising the resource limitations for your web server (for example add more worker processes). 
  Please consult your web server docs. If this also doesn't resolve the issue:
- Set up an architecture with multiple web servers and load balancer to handle the traffic for your site.

</details>

<details>
<summary>Optimize Databases </summary>

The response speed is dependent on database optimization. As you first set up a website, the
database responds quickly to queries. As time passes, the database accumulates information. The
compilation results in massive amounts of stored data and might slow down response times.

If you manage your database with MySQL, this blogpost proposes ways to [tune MySQL operations](https://www.cloudways.com/blog/mysql-performance-tuning/).
</details>

<details>
<summary>Configure Caching </summary>

Caching ensures fast delivery to visitors. Without caching, a browser requests assets from the
server each time a page loads instead of accessing them from a local or intermediary cache.

To enable caching on your server, refer to the respective documentation:
- [NGINX caching guide](https://www.nginx.com/blog/nginx-caching-guide/)
- [Apache caching guide](https://httpd.apache.org/docs/2.4/caching.html)

</details>
