# Netdata Cloud On-Prem Installation

This installation guide assumes the prerequisites for installing Netdata Cloud On-Prem as satisfied. For more information please refer to the [requirements documentation](/docs/netdata-cloud/netdata-cloud-on-prem/README.md#requirements).

## Installation Requirements

The following components are required to install Netdata Cloud On-Prem:

- **AWS** CLI
- **Helm** version 3.12+ with OCI Configuration (explained in the installation section)
- **Kubectl**

The minimum requirements for Netdata-Cloud are:
- 4 CPU cores
- 15GiB of memory
- Cloud services are ephemeral

Requirements for non-production Dependencies helm chart:
- 8 CPU cores
- 14GiB of memory
- 160GiB for PVCs (SSD)

> **_NOTE:_** Values for each component may vary depending on the type of load. The most intensive task, compute-wise, that the cloud needs to perform is the initial sync of directly connected agents. Requirements testing was done with 1,000 nodes directly connected to the cloud. If you plan on spawning hundreds of new nodes in a few minutes time window, Postgres is going to be the first bottleneck. For example, a 2 vCPU / 8 GiB memory / 1k IOPS database can handle 1,000 nodes without any problems if your environment is fairly steady, adding nodes in 10-30 batches (directly connected).

## Preparations for Installation

### Configure AWS CLI

Install [AWS CLI](https://docs.aws.amazon.com/cli/latest/userguide/getting-started-install.html).

There are 2 options for configuring `aws cli` to work with the provided credentials. The first one is to set the environment variables:

```bash
export AWS_ACCESS_KEY_ID=<your_secret_id>
export AWS_SECRET_ACCESS_KEY=<your_secret_key>
```

The second one is to use an interactive shell:

```bash
aws configure
```

### Configure helm to use secured ECR repository

Using `aws` command we will generate a token for helm to access the secured ECR repository:

```bash
aws ecr get-login-password --region us-east-1 | helm registry login --username AWS --password-stdin 362923047827.dkr.ecr.us-east-1.amazonaws.com
```

After this step you should be able to add the repository to your helm or just pull the helm chart:

```bash
helm pull oci://362923047827.dkr.ecr.us-east-1.amazonaws.com/netdata-cloud-dependency --untar #optional
helm pull oci://362923047827.dkr.ecr.us-east-1.amazonaws.com/netdata-cloud-onprem --untar
```

Local folders with the newest versions of helm charts should appear on your working dir.

## Installation

Netdata provides access to two helm charts:

1. `netdata-cloud-dependency` - required applications for `netdata-cloud-onprem`.
2. `netdata-cloud-onprem` - the application itself + provisioning

### netdata-cloud-dependency

This helm chart is designed to install the necessary applications:

- Redis
- Elasticsearch
- EMQX
- Apache Pulsar
- PostgreSQL
- Traefik
- Mailcatcher
- k8s-ecr-login-renew
- kubernetes-ingress

Although we provide an easy way to install all these applications, we expect users of Netdata Cloud On-Prem to provide production quality versions for them. Therefore, every configuration option is available through `values.yaml` in the folder that contains your netdata-cloud-dependency helm chart. All configuration options are described in `README.md` which is a part of the helm chart.

Each component can be enabled/disabled individually. It is done by true/false switches in `values.yaml`. This way, it is easier to migrate to production-grade components gradually.

Unless you prefer otherwise, `k8s-ecr-login-renew` is responsible for calling out the `AWS API` for token regeneration.  This token is then injected into the secret that every node is using for authentication with secured ECR when pulling the images.

The default setting in `values.yaml` of `netdata-cloud-onprem` - `.global.imagePullSecrets` is configured to work out of the box with the dependency helm chart.

For helm chart installation - save your changes in `values.yaml` and execute:

```shell
cd [your helm chart location]
helm upgrade --wait --install netdata-cloud-dependency -n netdata-cloud --create-namespace -f values.yaml .
```

Keep in mind that `netdata-cloud-dependency` is provided only as a proof of concept. Users installing Netdata Cloud On-Prem should properly configure these components.

### netdata-cloud-onprem

Every configuration option is available in `values.yaml` in the folder that contains your `netdata-cloud-onprem` helm chart. All configuration options are described in the `README.md` which is a part of the helm chart.

#### Installing Netdata Cloud On-Prem

```shell
cd [your helm chart location]
helm upgrade --wait --install netdata-cloud-onprem -n netdata-cloud --create-namespace -f values.yaml .
```

##### Important notes

1. Installation takes care of provisioning the resources with migration services.

2. During the first installation, a secret called the `netdata-cloud-common` is created. It contains several randomly generated entries. Deleting helm chart is not going to delete this secret, nor reinstalling the whole On-Prem, unless manually deleted by kubernetes administrator. The content of this secret is extremely relevant - strings that are contained there are essential parts of encryption. Losing or changing the data that it contains will result in data loss.

## Short description of Netdata Cloud microservices

#### cloud-accounts-service

Responsible for user registration & authentication. Manages user account information.

#### cloud-agent-data-ctrl-service

Forwards request from the cloud to the relevant agents.
The requests include:
- Fetching chart metadata from the agent
- Fetching chart data from the agent
- Fetching function data from the agent

#### cloud-agent-mqtt-input-service

Forwards MQTT messages emitted by the agent related to the agent entities to the internal Pulsar broker. These include agent connection state updates.

#### cloud-agent-mqtt-output-service

Forwards Pulsar messages emitted in the cloud related to the agent entities to the MQTT broker. From there, the messages reach the relevant agent.

#### cloud-alarm-config-mqtt-input-service

Forwards MQTT messages emitted by the agent related to the alarm-config entities to the internal Pulsar broker.  These include the data for the alarm configuration as seen by the agent.

#### cloud-alarm-log-mqtt-input-service

Forwards MQTT messages emitted by the agent related to the alarm-log entities to the internal Pulsar broker. These contain data about the alarm transitions that occurred in an agent.

#### cloud-alarm-mqtt-output-service

Forwards Pulsar messages emitted in the cloud related to the alarm entities to the MQTT broker. From there, the messages reach the relevant agent.

#### cloud-alarm-processor-service

Persists latest alert statuses received from the agent in the cloud.
Aggregates alert statuses from relevant node instances.
Exposes API endpoints to fetch alert data for visualization on the cloud.
Determines if notifications need to be sent when alert statuses change and emits relevant messages to Pulsar.
Exposes API endpoints to store and return notification-silencing data.

#### cloud-alarm-streaming-service

Responsible for starting the alert stream between the agent and the cloud.
Ensures that messages are processed in the correct order, and starts a reconciliation process between the cloud and the agent if out-of-order processing occurs.

#### cloud-charts-mqtt-input-service

Forwards MQTT messages emitted by the agent related to the chart entities to the internal Pulsar broker. These include the chart metadata that is used to display relevant charts on the cloud.

#### cloud-charts-mqtt-output-service

Forwards Pulsar messages emitted in the cloud related to the charts entities to the MQTT broker. From there, the messages reach the relevant agent.

#### cloud-charts-service

Exposes API endpoints to fetch the chart metadata.
Forwards data requests via the `cloud-agent-data-ctrl-service` to the relevant agents to fetch chart data points.
Exposes API endpoints to call various other endpoints on the agent, for instance, functions.

#### cloud-custom-dashboard-service

Exposes API endpoints to fetch and store custom dashboard data.

#### cloud-environment-service

Serves as the first contact point between the agent and the cloud.
Returns authentication and MQTT endpoints to connecting agents.

#### cloud-feed-service

Processes incoming feed events and stores them in Elasticsearch.
Exposes API endpoints to fetch feed events from Elasticsearch.

#### cloud-frontend

Contains the on-prem cloud website. Serves static content.

#### cloud-iam-user-service

Acts as a middleware for authentication on most of the API endpoints. Validates incoming token headers, injects the relevant ones, and forwards the requests.

#### cloud-metrics-exporter

Exports various metrics from an On-Prem Cloud installation. Uses the Prometheus metric exposition format.

#### cloud-netdata-assistant

Exposes API endpoints to fetch a human-friendly explanation of various netdata configuration options, namely the alerts.

#### cloud-node-mqtt-input-service

Forwards MQTT messages emitted by the agent related to the node entities to the internal Pulsar broker. These include the node metadata as well as their connectivity state, either direct or via parents.

#### cloud-node-mqtt-output-service

Forwards Pulsar messages emitted in the cloud related to the charts entities to the MQTT broker. From there, the messages reach the relevant agent.

#### cloud-notifications-dispatcher-service

Exposes API endpoints to handle integrations.
Handles incoming notification messages and uses the relevant channels(email, slack...) to notify relevant users.

#### cloud-spaceroom-service

Exposes API endpoints to fetch and store relations between agents, nodes, spaces, users, and rooms.
Acts as a provider of authorization for other cloud endpoints.
Exposes API endpoints to authenticate agents connecting to the cloud.
