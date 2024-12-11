# Netdata Agent Installation

Netdata is very flexible and can be used to monitor all kinds of infrastructure. Read more about possible [Deployment guides](/docs/deployment-guides/README.md) to understand what better suites your needs.

## Install through Netdata Cloud

The easiest way to install Netdata on your system is via Netdata Cloud, to do so:

1. Sign in to <https://app.netdata.cloud/>.
2. Select a [Space](/docs/netdata-cloud/organize-your-infrastructure-invite-your-team.md#spaces), and click the "Connect Nodes" prompt, which will show the install command for your platform of choice.
3. Copy and paste the script into your node's terminal, and run it.

Once Netdata is installed, you can see the node live in your Netdata Space and charts in the [Metrics tab](/docs/dashboards-and-charts/metrics-tab-and-single-node-tabs.md).

## Anonymous statistics

Netdata collects anonymous usage information by default and sends it to a self-hosted PostHog instance within the Netdata infrastructure. Read about the information collected on our [anonymous statistics](/docs/netdata-agent/configuration/anonymous-telemetry-events.md) documentation page.

The usage statistics are _vital_ for us, as we use them to discover bugs and prioritize new features. We thank you for
_actively_ contributing to Netdata's future.
