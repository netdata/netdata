# Netdata Cloud On-Prem Installation

## System Requirements

| Component                  | Details                                                                                                                                        |
|:---------------------------|:-----------------------------------------------------------------------------------------------------------------------------------------------|
| **Kubernetes**             | - Version 1.23 or newer<br/>- Metrics server installed (for autoscaling)<br/>- Default storage class configured (SSD-based preferred)          |
| **TLS certificate**        | - Single certificate for all endpoints, or separate certificates for frontend, API, and MQTT<br/>- Must be trusted by all connecting entities. |
| **Netdata Cloud Services** | - 4 CPU cores<br/>- 15GiB memory<br/>- Note: Cloud services are ephemeral                                                                      |
| **Third-Party Services**   | - 8 CPU cores<br/>- 14GiB memory<br/>- 160GiB SSD storage for PVCs                                                                             |

> **Note**:
>
> These requirements were tested with 1,000 directly connected nodes.
> Resource needs may vary based on your workload.
> The initial sync of directly connected Agents is the most compute-intensive operation.
> For example, a Postgres instance with 2 vCPU, 8GiB memory, and 1k IOPS can handle 1,000 nodes in a steady environment when adding nodes in batches of 10–30.

## Required Components

### Third-Party Services

All components below are included in the `netdata-cloud-dependency` package:

| Component              | Version | Purpose                             |
|------------------------|---------|-------------------------------------|
| **PostgreSQL**         | 13.7    | Main metadata database              |
| **EMQX**               | 5.11    | MQTT Broker for Agent communication |
| **Apache Pulsar**      | 2.10+   | Inter-container message broker      |
| **Traefik**            | 2.11.x  | Internal API Gateway                |
| **Elasticsearch**      | 8.8.x   | Events feed storage                 |
| **Redis**              | 6.2     | Caching layer                       |
| **Ingress Controller** | -       | HTTPS support                       |
| **imagePullSecret**    | -       | Secured ECR repository access       |

> **Important**:
>
> The provided dependency versions require additional configuration for production use.
> Customers should configure these applications according to their production requirements and policies.

### Installation Tools

- [AWS CLI](https://docs.aws.amazon.com/cli/latest/userguide/getting-started-install.html)
- Helm (version 3.12+ with OCI Configuration)
- Kubectl

## Installation

1. **AWS CLI Configuration**

   Configure AWS credentials using either environment variables:

   ```bash
   export AWS_ACCESS_KEY_ID=<your_secret_id>
   export AWS_SECRET_ACCESS_KEY=<your_secret_key>
   ```

   Or through interactive setup:

   ```bash
   aws configure
   ```

2. **Configure Helm for ECR Access**

   Generate token for ECR access:

   ```bash
   aws ecr get-login-password --region us-east-1 | helm registry login --username AWS --password-stdin 362923047827.dkr.ecr.us-east-1.amazonaws.com
   ```

3. **Pull Required Helm Charts**

   ```bash
   helm pull oci://362923047827.dkr.ecr.us-east-1.amazonaws.com/netdata-cloud-dependency --untar  # Optional
   helm pull oci://362923047827.dkr.ecr.us-east-1.amazonaws.com/netdata-cloud-onprem --untar
   ```

   The charts will be extracted to your current working directory.

4. **Install Dependencies**

   The `netdata-cloud-dependency` chart installs all required third-party applications. While we provide this for easy setup, **production environments should use their own configured versions of these components**:

    - Configure the installation by editing `values.yaml` in your `netdata-cloud-dependency` chart directory.
    - Install the dependencies:
      ```bash
      cd [your helm chart location]
      helm upgrade --wait --install netdata-cloud-dependency -n netdata-cloud --create-namespace -f values.yaml .
      ```

5. **Install Netdata Cloud On-Prem**

    - Configure the installation by editing `values.yaml` in your `netdata-cloud-onprem` chart directory.
    - Install the application:
      ```bash
      cd [your helm chart location]
      helm upgrade --wait --install netdata-cloud-onprem -n netdata-cloud --create-namespace -f values.yaml .
      ```

   > **Important**:
   >
   > Installation includes resource provisioning with migration services.
   >
   > During the first installation, a `netdata-cloud-common` secret is created containing critical encryption data. This secret persists through reinstalls and should never be deleted, as this will result in data loss.

## Architecture Components

<details><summary>View detailed microservices description</summary>

| Microservice                           | Description                                                                                                                                                                                                                                                                                                                                                                                        |
|:---------------------------------------|:---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| cloud-accounts-service                 | Handles user registration & authentication                                                                                                                                                                                                                                                                                                                                                         |
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
