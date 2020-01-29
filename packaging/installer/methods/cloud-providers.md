# Install Netdata on cloud providers

Netdata is fully compatible with popular cloud providers like Google Cloud Platform (GCP), Amazon Web Services (AWS),
Azure, and others. You can install Netdata on cloud instances to monitor the apps/services running there, or use
multiple instances in a [master/slave streaming](../../../streaming/README.md) configuration.

In some cases, using Netdata on these cloud providers requires unique installation or configuration steps. This page
aims to document some of those steps for popular cloud providers.

If you find new issues specific to a cloud provider, or would like to help clarify the correct workaround, please
[create an
issue](https://github.com/netdata/netdata/issues/new?labels=feature+request%2C+needs+triage&template=feature_request.md)
with your process and instructions on using the provider's interface to complete the workaround.

-   [Recommended installation method](#recommended-installation-method-for-cloud-providers)
-   [Add a firewall rule to access Netdata's dashboard](#add-a-firewall-rule-to-access-netdatas-dashboard)

## Recommended installation method for cloud providers

The best installation method depends on the instance's operating system, distribution, and version. For Linux instances,
we recommend either the [`kickstart.sh` automatic installation script](kickstart.md) or [.deb/.rpm
packages](packages.md).

To see the full list of approved methods for each operating system/version we support, see our [distribution
matrix](../../DISTRIBUTIONS.md). That table will guide you to the various supported methods for your cloud instance.

If you have issues with Netdata after installation, look to the sections below to find any post-installation
configuration steps for your cloud provider.

## Add a firewall rule to access Netdata's dashboard

If you cannot access Netdata's dashboard on your cloud instance via `http://HOST:19999`, and instead get an error page
from your browser that says, "This site can't be reached" (Chrome) or "Unable to connect" (Firefox), you may need to
configure your cloud provider's firewall.

Cloud providers often create network-level firewalls that run separately from the instance itself. Both AWS and Google
Cloud Platform calls them Virtual Private Cloud (VPC) networks. These firewalls can apply even if you've disabled
firewalls on the instance itself. Because you can modify these firewalls only via the cloud provider's web interface,
it's easy to overlook them when trying to configure and access Netdata's dashboard.

You can often confirm a firewall issue by querying the dashboard while connected to the instance via SSH: `curl
http://localhost:19999/api/v1/info`. If you see JSON output, Netdata is running properly. If you try the same `curl`
command from a remote IP, and it fails, it's likely that a firewall is blocking your requests.

### Google Cloud Platform (GCP)

To add a firewall rule, go to the [Firewall rules page](https://console.cloud.google.com/networking/firewalls/list) and
click **Create firewall rule**. Read GCP's [firewall documentation](https://cloud.google.com/vpc/docs/using-firewalls)
for specific instructions on how to create a new firewall rule. The following configuration has previously worked for
Netdata running on GCP instances ([#7786](https://github.com/netdata/netdata/issues/7786)):

```conf
Name: <name>
Type: Ingress
Targets: <name-tag>
Filters: 0.0.0.0/0
Protocols/ports: 19999
Action: allow
Priority: 1000
```
