# Install Netdata on AWS

Netdata is fully compatible with Amazon Web Services (AWS).
You can install Netdata on cloud instances to monitor the apps/services running there, or use multiple instances in a [parent-child streaming](/src/streaming/README.md) configuration.

## Recommended installation method

The best installation method depends on the instance's operating system, distribution, and version. For Linux instances, we recommend the [`kickstart.sh` automatic installation script](/packaging/installer/methods/kickstart.md).

If you have issues with Netdata after installation, look to the sections below to find the issue you're experiencing, followed by the solution for your provider.

## Post-installation configuration

### Add a firewall rule to access Netdata's dashboard

If you cannot access Netdata's dashboard on your cloud instance via `http://HOST:19999`, and instead get an error page
from your browser that says, "This site can't be reached" (Chrome) or "Unable to connect" (Firefox), you may need to configure your cloud provider's firewall.

Cloud providers often use network-level firewalls, called Virtual Private Cloud (VPC) networks (e.g., AWS and Google Cloud), separate from the instance. These firewalls can still apply even if the instance's firewall is disabled. Since they can only be modified via the cloud provider's web interface, they are easy to overlook when configuring or accessing Netdata’s dashboard.

You can often confirm a firewall issue by querying the dashboard while connected to the instance via SSH: `curl
http://localhost:19999/api/v1/info`. If you see JSON output, Netdata is running properly. If you try the same `curl`
command from a remote system, and it fails, it's likely that a firewall is blocking your requests.

Another option is to put Netdata behind web server, which will proxy requests through standard HTTP/HTTPS ports
(80/443), which are likely already open on your instance. We have a number of guides available:

- [Apache](/docs/netdata-agent/configuration/running-the-netdata-agent-behind-a-reverse-proxy/Running-behind-apache.md)
- [Nginx](/docs/netdata-agent/configuration/running-the-netdata-agent-behind-a-reverse-proxy/Running-behind-nginx.md)
- [Caddy](/docs/netdata-agent/configuration/running-the-netdata-agent-behind-a-reverse-proxy/Running-behind-caddy.md)
- [HAProxy](/docs/netdata-agent/configuration/running-the-netdata-agent-behind-a-reverse-proxy/Running-behind-haproxy.md)
- [lighttpd](/docs/netdata-agent/configuration/running-the-netdata-agent-behind-a-reverse-proxy/Running-behind-lighttpd.md)

Sign in to the [AWS console](https://console.aws.amazon.com/) and navigate to the EC2 dashboard. Click on the **Security
Groups** link in the navigation, beneath the **Network & Security** heading. Find the Security Group your instance
belongs to, and either right-click on it or click the **Actions** button above to see a dropdown menu with **Edit
inbound rules**.

Add a new rule with the following options:

```text
Type: Custom TCP
Protocol: TCP
Port Range: 19999
Source: Anywhere
Description: Netdata
```

You can also choose **My IP** as the source if you prefer.

Click **Save** to apply your new inbound firewall rule.