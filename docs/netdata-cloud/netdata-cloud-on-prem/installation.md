# Netdata Cloud On-Prem Installation

## Prerequisites

The following components are required to run the On-Prem Cloud (OPC):

| component                                        | description                                                                                                                                                                                         |
|:-------------------------------------------------|:----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| **Kubernetes cluster**                           | v1.23+                                                                                                                                                                                              |
| **Kubernetes metrics server**                    | Used for autoscaling                                                                                                                                                                                |
| **TLS certificate**                              | Used for secure connections. A single endpoint is required but there is an option to split the frontend, API, and MQTT endpoints. The certificate must be trusted by all entities connecting to it. |
| Default **storage class configured and working** | Persistent volumes based on SSDs are preferred                                                                                                                                                      |

The following 3rd party components are used, which can be pulled with the `netdata-cloud-dependency` package we provide:

| component              | description                                                       |
|:-----------------------|:------------------------------------------------------------------|
| **Ingress controller** | Supporting HTTPS                                                  |
| **PostgreSQL**         | v13.7, the main database for all metadata the OPC maintains       |
| **EMQX**               | v5.11, MQTT Broker that allows Agents to send messages to the OPC |
| **Apache Pulsar**      | v2.10+, message broker for inter-container communication          |
| **Traefik**            | v2.7.x, the internal API Gateway                                  |
| **Elasticsearch**      | v8.8.x, stores the events feed                                    |
| **Redis**              | v6.2, used for caching                                            |
| **imagePullSecret**    | our ECR repos are secured                                         |

> **Note**
>
> Keep in mind that the pulled versions are not configured properly for production use.
>
> Customers of OPC are expected to configure these applications according to their needs and policies for production use.

**The following components are required for installation:**

- AWS CLI
- Helm version 3.12+ with OCI Configuration (explained in the installation section)
- Kubectl

**Minimum requirements:**

- 4 CPU cores
- 15GiB of memory
- Cloud services are ephemeral

**The requirements for the non-production Dependencies helm chart:**

- 8 CPU cores
- 14GiB of memory
- 160GiB for PVCs (SSD)

> **Note**
>
> Values for each component may vary depending on the type of load. The most compute-intensive task that the OPC needs to perform is the initial sync of directly connected Agents.
>
> The testing for these requirements was conducted with 1,000 nodes directly connected to the OPC.
>
> If you plan on spawning hundreds of new nodes within a few minutes, Postgres will be the first bottleneck. For example, a 2 vCPU / 8 GiB memory / 1k IOPS database can handle 1,000 nodes without any problems if your environment is fairly steady, adding nodes in batches of 10-30 (directly connected).

## Preparation

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

### `netdata-cloud-dependency`

This helm chart is designed to install the necessary applications mentioned in the prerequisites.

Although we provide an easy way to install all these applications, we expect users of OPC to provide production quality versions for them. Therefore, every configuration option is available through `values.yaml` in the folder that contains your netdata-cloud-dependency helm chart. All configuration options are described in the `README.md` which is a part of the helm chart.

Each component can be enabled/disabled individually. It is done by true/false switches in `values.yaml`. This way it is easier to migrate to production-grade components gradually.

Unless you prefer otherwise, `k8s-ecr-login-renew` is responsible for calling out the `AWS API` for token regeneration.  This token is then injected into the secret that every node is using for authentication with secured ECR when pulling the images.

The default setting in `values.yaml` of `netdata-cloud-onprem` - `.global.imagePullSecrets` is configured to work out of the box with the dependency helm chart.

For helm chart installation - save your changes in `values.yaml` and execute:

```shell
cd [your helm chart location]
helm upgrade --wait --install netdata-cloud-dependency -n netdata-cloud --create-namespace -f values.yaml .
```

Keep in mind that `netdata-cloud-dependency` is provided only as a proof of concept. Users installing OPC should properly configure these components.

### `netdata-cloud-onprem`

Every configuration option is available in `values.yaml` in the folder that contains your `netdata-cloud-onprem` helm chart. All configuration options are described in the `README.md` which is a part of the helm chart.

**Afterwards, you can install the OPC:**

```shell
cd [your helm chart location]
helm upgrade --wait --install netdata-cloud-onprem -n netdata-cloud --create-namespace -f values.yaml .
```

> **Important**
>
> 1. Installation takes care of provisioning the resources with migration services.
>
> 2. During the first installation, a secret called the `netdata-cloud-common` is created. It contains several randomly generated entries. Deleting helm chart is not going to delete this secret, nor reinstalling the whole OPC, unless manually deleted by a kubernetes administrator. The content of this secret is extremely important - strings that are contained there are essential parts of encryption. Losing or changing the data that it contains will result in data loss.

## Short description of the microservices

<details><summary>details</summary>

| microservice                           | description                                                                                                                                                                                                                                                                                                                                                                                        |
|:---------------------------------------|:---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| cloud-accounts-service                 | Responsible for user registration & authentication                                                                                                                                                                                                                                                                                                                                                 |
| cloud-agent-data-ctrl-service          | Forwards request from the OCP to the relevant Agents. The requests include fetching Chart metadata, Chart data and Function data from the Agents.                                                                                                                                                                                                                                                  |
| cloud-agent-mqtt-input-service         | Forwards MQTT messages emitted by the Agent to the internal Pulsar broker. They are related to the Agent entities and include Agent connection state updates.                                                                                                                                                                                                                                      |
| cloud-agent-mqtt-output-service        | Forwards Pulsar messages emitted on the OCP to the MQTT broker. They are related to the Agent entities. From there, the messages reach the relevant Agent.                                                                                                                                                                                                                                         |
| cloud-alarm-config-mqtt-input-service  | Forwards MQTT messages emitted by the Agent to the internal Pulsar broker. They related to the alarm-config entities like data for the alarm configuration as seen by the Agent.                                                                                                                                                                                                                   |
| cloud-alarm-log-mqtt-input-service     | Forwards MQTT messages emitted by the Agent to the internal Pulsar broker. They are related to the alarm-log entities containing data about the alarm transitions that occurred in an Agent.                                                                                                                                                                                                       |
| cloud-alarm-mqtt-output-service        | Forwards Pulsar messages emitted in the Cloud to the MQTT broker. They are related to the alarm entities and from there, the messages reach the relevant Agent.                                                                                                                                                                                                                                    |
| cloud-alarm-processor-service          | Persists latest Alert status received from the Agent in the OCP<br/>Aggregates Alert statuses from relevant node instances<br/>Exposes API endpoints to fetch Alert data for visualization on the Cloud<br/>Determines if notifications need to be sent when Alert statuses change and emits relevant messages to Pulsar<br/>Exposes API endpoints to store and return notification-silencing data |
| cloud-alarm-streaming-service          | Responsible for starting the Alert stream between the Agent and the OCP<br/>Ensures that messages are processed in the correct order, and starts a reconciliation process between the Cloud and the Agent if out-of-order processing occurs                                                                                                                                                        |
| cloud-charts-mqtt-input-service        | Forwards MQTT messages emitted by the Agent related to the chart entities to the internal Pulsar broker. These include the chart metadata that is used to display relevant charts on the Cloud.                                                                                                                                                                                                    |
| cloud-charts-mqtt-output-service       | Forwards Pulsar messages emitted in the Cloud related to the charts entities to the MQTT broker. From there, the messages reach the relevant Agent.                                                                                                                                                                                                                                                |
| cloud-charts-service                   | Exposes API endpoints to fetch the chart metadata<br/>Forwards data requests via the `cloud-agent-data-ctrl-service` to the relevant Agents to fetch chart data points<br/>Exposes API endpoints to call various other endpoints on the Agent, for instance, functions                                                                                                                             |
| cloud-custom-dashboard-service         | Exposes API endpoints to fetch and store custom dashboard data                                                                                                                                                                                                                                                                                                                                     |
| cloud-environment-service              | Serves as the first contact point between the Agent and the OCP<br/>Returns authentication and MQTT endpoints to connecting Agents                                                                                                                                                                                                                                                                 |
| cloud-feed-service                     | Processes incoming feed events and stores them in Elasticsearch<br/>Exposes API endpoints to fetch feed events from Elasticsearch                                                                                                                                                                                                                                                                  |
| cloud-frontend                         | Contains the OCP website. Serves static content.                                                                                                                                                                                                                                                                                                                                                   |
| cloud-iam-user-service                 | Acts as a middleware for authentication on most of the API endpoints<br/>Validates incoming token headers, injects the relevant ones, and forwards the requests                                                                                                                                                                                                                                    |
| cloud-metrics-exporter                 | Exports various metrics from an OCP installation<br/>Uses the Prometheus metric exposition format                                                                                                                                                                                                                                                                                                  |
| cloud-netdata-assistant                | Exposes API endpoints to fetch a human-friendly explanation of various Netdata configuration options, namely the Alerts.                                                                                                                                                                                                                                                                           |
| cloud-node-mqtt-input-service          | Forwards MQTT messages emitted by the Agent related to the node entities to the internal Pulsar broker<br/>These include the node metadata as well as their connectivity state, either direct or via Parents                                                                                                                                                                                       |
| cloud-node-mqtt-output-service         | Forwards Pulsar messages emitted in the OCP related to the charts entities to the MQTT broker<br/>From there, the messages reach the relevant Agent                                                                                                                                                                                                                                                |
| cloud-notifications-dispatcher-service | Exposes API endpoints to handle integrations<br/>Handles incoming notification messages and uses the relevant channels(email, slack...) to notify relevant users                                                                                                                                                                                                                                   |
| cloud-spaceroom-service                | Exposes API endpoints to fetch and store relations between Agents, nodes, spaces, users, and rooms<br/>Acts as a provider of authorization for other Cloud endpoints<br/>Exposes API endpoints to authenticate Agents connecting to the Cloud                                                                                                                                                      |

</details>
