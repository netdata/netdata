# Netdata Agent Installation

Netdata is very flexible and can be used to monitor all kinds of infrastructure. Read more about possible [Deployment guides](/docs/deployment-guides/README.md) to understand what better suites your needs.

## Install through Netdata Cloud

The easiest way to install Netdata on your system is via Netdata Cloud, to do so:

1. Sign in to <https://app.netdata.cloud/>.
2. Select a [Space](/docs/netdata-cloud/organize-your-infrastructure-invite-your-team.md#overview), and click the "Connect Nodes" prompt, which will show the install command for your platform of choice.
3. Copy and paste the script into your node's terminal, and run it.

Once Netdata is installed, you can see the node live in your Netdata Space and charts in the [Metrics tab](/docs/dashboards-and-charts/metrics-tab-and-single-node-tabs.md).

## Install Directly on Your System

You can also install Netdata directly without connecting to Netdata Cloud. Choose your platform below:

### Operating Systems

**Linux** - Install on any Linux distribution
[→ Install on Linux](/packaging/installer/methods/kickstart.md)

**Windows** - Native Windows monitoring agent
[→ Install on Windows](/packaging/windows/WINDOWS_INSTALLER.md)

**macOS** - Monitor your Mac systems
[→ Install on macOS](/packaging/installer/methods/macos.md)

**FreeBSD** - Install on FreeBSD systems
[→ Install on FreeBSD](/packaging/installer/methods/freebsd.md)

### Containerized Environments

**Docker** - Run Netdata in Docker containers
[→ Install with Docker](/packaging/docker/README.md)

**Kubernetes** - Deploy across Kubernetes clusters
[→ Install on Kubernetes](/packaging/installer/methods/kubernetes.md)

### Network Appliances

**pfSense** - Monitor pfSense firewalls
[→ Install on pfSense](/packaging/installer/methods/pfsense.md)

**Synology** - Install on Synology NAS devices
[→ Install on Synology](/packaging/installer/methods/synology.md)

### Automation & Cloud Platforms

**Ansible** - Automate deployment with Ansible
[→ Install with Ansible](/packaging/installer/methods/ansible.md)

**AWS** - Deploy on Amazon Web Services
[→ Install on AWS](/packaging/installer/methods/aws.md)

**Azure** - Deploy on Microsoft Azure
[→ Install on Azure](/packaging/installer/methods/azure.md)

**GCP** - Deploy on Google Cloud Platform
[→ Install on GCP](/packaging/installer/methods/gcp.md)

## Verify Your Installation

If the installation completed but nothing appears in your dashboard, work through this checklist:

1. **Confirm the Agent service is running.** [Check the service status](/docs/netdata-agent/start-stop-restart.md#check-status) for your platform and confirm the Agent is reachable at `http://NODE:19999`.

2. **Open the right dashboard for your setup.** For connected nodes, check [Netdata Cloud](https://app.netdata.cloud/). For a node that is not connected to Netdata Cloud, or for local-only access, open the local Agent dashboard at `http://NODE:19999`.

3. **If the node is missing from Netdata Cloud, verify it was connected.** Connecting links the Agent to your Netdata Cloud Space. See [Connect Agent to Cloud](/src/claim/README.md#connect-agent-to-cloud) for the steps. If the Agent was already connected but the node still does not appear, see [Troubleshooting](/src/claim/README.md#troubleshooting) to check the Cloud connection status.

4. **Allow inbound TCP port 19999 for local dashboard access from another machine.** The host firewall on the node must permit connections to the Agent's port. If the local dashboard does not load from a remote browser, confirm port 19999 is open on the node.

## What can I monitor with Netdata?

Netdata can monitor virtually any system, application, or service. Discover what you can monitor and explore our extensive integrations catalog.

[→ Explore what Netdata can monitor](/src/collectors/COLLECTORS.md)
