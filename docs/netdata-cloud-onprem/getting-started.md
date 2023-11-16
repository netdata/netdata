# Getting started with Netdata Cloud On-Prem
Helm chart is designed for kubernetes to run as the local equivalent of the netdata.cloud public offering.

## Requirements
#### Install host:
- [AWS CLI](https://docs.aws.amazon.com/cli/latest/userguide/getting-started-install.html)
- [Helm](https://helm.sh/docs/intro/install/) version 3.12+ with OCI Configuration (explained in installation section)
- [Kubectl](https://kubernetes.io/docs/tasks/tools/)

#### Kubernetes requirements:
- Kubernetes cluster version 1.23+
- Kubernetes metrics server (For autoscaling)
- TLS certificate for Netdata Cloud On-Prem
- Ingress controller to support HTTPS `*`
- PostgreSQL version 13.7 `*` (Main persistent data app)
- EMQX version 5.11 `*` (MQTT Broker that allows Agents to send messages to the On-Prem Cloud)
- Apache Pulsar version 2.10+ `*` (Central communication hub. Applications exchange messages through Pulsar)
- Traefik version 2.7.x `*` (Internal communication - API Gateway)
- Elastic Search version 8.8.x `*` (Holds Feed)
- Redis version 6.2 `*` (Cache)
- Some form of generating imagePullSecret `*` (Our ECR repos are secured)
- Default storage class configured and working (Persistent volumes based on SSDs are preferred)
`*` - available in dependencies helm chart for PoC applications.


## Pulling the helm chart
Helm chart for the Netdata Cloud On-Prem installation on Kubernetes is available at ECR registry.
ECR registry is private, so you need to login first. Credentials are sent by our Product Team. If you do not have them, please contact our Product Team - info@netdata.cloud.

#### Configure AWS CLI
Machine used for helm chart installation will also need [AWS CLI installed](https://docs.aws.amazon.com/cli/latest/userguide/getting-started-install.html).
There are 2 options of configuring `aws` cli to work with provided credentials. First one is to set the environment variables:
```bash
export AWS_ACCESS_KEY_ID=<your_secret_id>
export AWS_SECRET_ACCESS_KEY=<your_secret_key>
```

Second one is to use an interactive shell:
```bash
aws configure
```

#### Configure helm to use secured ECR repository
Using `aws` command we will generate token for helm to access secured ECR repository:
```bash
aws ecr get-login-password --region us-east-1 | helm registry login --username AWS --password-stdin 362923047827.dkr.ecr.us-east-1.amazonaws.com/netdata-cloud-onprem
```

After this step you should be able to add the repository to your helm or just pull the helm chart:
```bash
helm pull oci://362923047827.dkr.ecr.us-east-1.amazonaws.com/netdata-cloud-dependency --untar #optional
helm pull oci://362923047827.dkr.ecr.us-east-1.amazonaws.com/netdata-cloud-onprem --untar
```

Local folders with newest versions of helm charts should appear on your working dir.

## Installation

Netdata provides access to two helm charts:
1. netdata-cloud-dependency - required applications for netdata-cloud-onprem. Not for production use.
2. netdata-cloud-onprem - the application itself + provisioning

### netdata-cloud-dependency

Entire helm chart is designed around the idea that it allows to install all of the necessary applications:
- redis
- elasticsearch
- emqx
- pulsar
- postgresql
- traefik
- mailcatcher
- k8s-ecr-login-renew
- kubernetes-ingress

Each component can be enabled/disabled individually. It is done by true/false switches in `values.yaml`.
Unless you prefer different solution to the problem, `k8s-ecr-login-renew` is responsible for calling out the `AWS API` for token regeneration. This token is then injected into the secret that every node is using for authentication with secured ECR when pulling the images.
Default setting in `values.yaml` of `netdata-cloud-onprem` - `.global.imagePullSecrets` is configured to work out of the box with the dependency helm chart.

For helm chart installation - save your changes in `values.yaml` and execute:
```shell
cd [your helm chart location]
helm upgrade --wait --install netdata-cloud-dependency -n netdata-cloud --create-namespace -f values.yaml .
```

#### Manual dependency configuration options for production usage
##### EMQX
1. Make sure setup meeds your HA (High Avability) requirements.
2. Environment variables to set:
  ```
  EMQX_BROKER__SHARED_SUBSCRIPTION_GROUP__cloudnodemqttinput__STRATEGY        = hash_clientid
  EMQX_BROKER__SHARED_SUBSCRIPTION_GROUP__cloudagentmqttinput__STRATEGY       = hash_clientid
  EMQX_BROKER__SHARED_SUBSCRIPTION_GROUP__cloudalarmlogmqttinput__STRATEGY    = hash_clientid
  EMQX_BROKER__SHARED_SUBSCRIPTION_GROUP__cloudalarmconfigmqttinput__STRATEGY = hash_clientid
  EMQX_FORCE_SHUTDOWN__MAX_HEAP_SIZE                                          = 128MB
  EMQX_AUTHENTICATION__1__MECHANISM                                           = password_based
  EMQX_AUTHENTICATION__1__BACKEND                                             = built_in_database
  EMQX_AUTHENTICATION__1__USER_ID_TYPE                                        = username
  EMQX_AUTHENTICATION__1__ENABLE                                              = true
  EMQX_AUTHORIZATION__NO_MATCH                                                = deny
  EMQX_AUTHORIZATION__SOURCES__1__TYPE                                        = file
  EMQX_AUTHORIZATION__SOURCES__1__ENABLE                                      = false
  EMQX_AUTHORIZATION__SOURCES__2__ENABLE                                      = true
  EMQX_AUTHORIZATION__SOURCES__2__TYPE                                        = built_in_database
  EMQX_MQTT__MAX_PACKET_SIZE                                                  = 5MB
  ```
3. Make sure `Values.global.emqx.provisioning` have all the data it needs. First password is the one you configured for your EMQX (needs to be an admin password). Second password username and password `Values.global.emqx.provisioning.users.netdata` is for the default user that services will use to contact EMQX's API.

##### Apache Pulsar
If you want to deploy Pulsar on the Kubernetes there is a ready to use helm chart available [here](https://pulsar.apache.org/docs/3.1.x/deploy-kubernetes/).
1. Authentiaction - only 1 method can be used at the time. Currently we support:
   - None (not recommended). Make sure everything is disabled in `Values.global.pulsar.authentication`
   - Basic auth - turn the feature on in `Values.global.pulsar.authentication.basic`, provide password for pulsar in the same section. Each service can be configured individually.
   - OAuth - configure section `Values.global.pulsar.authentication.oauth`. In this case applications need to also mount private key. Add it manually to the cluster and point `privateKeySecretName` to it. `privateKeySecretPath` is a mounting path for it.
2. Namespace we are using must be named `onprem` - this step is done by provisioning script during Netdata Cloud On-Prem installation.
3. You do not need to create Topics (by default they are creating themselves). Default creation method is to create non-partitioned topics. Partitioned topics can be used but there is no need in instalations for less than 30k Netdata Agent nodes. If you predict such big installation please contact us for further instructions.

##### Elastic Search
Elastic is going to be provisioned during the first installation. Make sure to setup Elastic in High Avability and configure network and credentials for the `cloud-feed-service` to be able to connect the Elastic instance.

##### Postgres
Postgres is provisioned automatically as well. Same it was with for example EMQX - `Values.global.postgres.provisioning` - first credentials for global admin, second one for creating the user called `dev`.
All the databases are created and assigned permissions during the first installation. `migrations` jobs that run every upgrade are there to apply schema and keep it up to date further further application changes.

##### Redis
We are using Redis in very basic and simple way. The only thing Netdata Cloud On-Prem needs is a password to Redis server. No additional provisioning is required since Redis can automatically create it's own "databases".

##### Traefik
We need traefik to:
1. Run in minimum 2 pods for HA.
2. Be able to utilize Netdata Cloud On-Prem namespace. We are deploying there `ingressroutes` and `middlewares`.
3. (Optional) Prometheus metrics can be enabled - Netdata Agent for Kubernetes (if installed) can scrape those metrics.

##### Ingress controller
This is the first point of contact for both the agents and the users. This is also configureable in 
General requirements:
1. Ingress for EMQX's passthrough:
   - Host from: `Values.global.public.cloudUrl`, port: `8083`, path: `/mqtt` - pointing to `emqx`'s service.
2. Ingress for the rest of communication.
   - Host from: `Values.global.public.cloudUrl`, port: `80`, path: `/` - pointing to `Values.global.ingress.traefikServiceName`.
   - Host from: `Values.global.public.apiUrl`, port: `80`, path: `/api` - pointing to `Values.global.ingress.traefikServiceName`.
3. Make sure you have ingress controller installed and correctly pointed to in `Values.global.ingress`. We ourselves are using [HAProxy Ingress Controller](https://github.com/haproxytech/kubernetes-ingress).


### netdata-cloud-onprem

Helm chart needs some basic configuration. Every configuration option is available through `values.yaml` in the folder that contains your netdata-cloud-onprem helm chart.

|Setting|Description|
|---|---|
|.global.netdata_cloud_license|This is section for license key that you will obtain from Product Team. **It is mandatory to provide correct key**|
|.global.pulsar|Section responsible for Apache Pulsar configuration. Default points to PoC installation from `netdata-cloud-dependency`|
|.global.emqx|Section responsible for EMQX configuration. Default points to PoC installation from `netdata-cloud-dependency`|
|.global.redis|Section responsible for Redis configuration. Default points to PoC installation from `netdata-cloud-dependency`|
|.global.postgresql|Section responsible for PostgreSQL configuration. Default points to PoC installation from `netdata-cloud-dependency`|
|.global.elastic|Section responsible for Elastic Search configuration. Default points to PoC installation from `netdata-cloud-dependency`|
|.global.oauth.github|Settings for login through GitHub. If not configured this option will not work at all|
|.global.oauth.google|Settings for login through Google account. Without configuration there is no option to login with Google Account|
|.global.mail.sendgrid|Netdata Cloud is able to send mails through sendgrid, this section allows for it's configuration|
|.global.mail.smtp|Section for SMTP server configuration. By default it points to the mailcatcher that is installed by dependency helm chart. To access emails without proper SMTP server, setup port forwarding to mailcatcher on port `1080` for webui. By default this is the only avaiable option to access the cloud|
|.global.ingress|Section responsible for Ingress configuration. After enabling this feature helm chart will create needed ingresses|
|.<APP_NAME>|Each netdata application have it's own section. You can tune services or passwords individually for each application. `<APP_NAME>.autoscaling` is useful when scaling for more performance. Short description of the applications avaiable below|


#### Installing Netdata Cloud On-Prem
```shell
cd [your helm chart location]
helm upgrade --wait --install netdata-cloud-onprem -n netdata-cloud --create-namespace -f values.yaml .
```

##### Important notes
1. Installation takes care of provisioning the resources with migration services.
1. During the first installation, a secret called the `netdata-cloud-common` is created. It contains several randomly generated entries. Deleting helm chart is not going to delete this secret, nor reinstalling whole onprem, unless manually deleted by kubernetes administrator. Content of this secret is extremely relevant - strings that are contained there are essential part of encryption. Loosing or changing data that it contains will result in data loss.

## Short description of services
#### cloud-accounts-service
Responsible for user registration & authentication. Manages user account information.
#### cloud-agent-data-ctrl-service
Forwards requests from the cloud to the relevant agents. 
The requests include:
* Fetching chart metadata from the agent
* Fetching chart data from the agent
* Fetching function data from the agent
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
Exposes API endpoints to store and return notification silencing data.
#### cloud-alarm-streaming-service
Responsible for starting the alert stream between the agent and the cloud.
Ensures that messages are processed in the correct order, starts a reconciliation process between the cloud and the agent if out of order processing occurs.
#### cloud-charts-mqtt-input-service
Forwards MQTT messages emitted by the agent related to the chart entities to the internal Pulsar broker. These include the chart metadata that is used to display relevant charts on the cloud.
#### cloud-charts-mqtt-output-service
Forwards Pulsar messages emitted in the cloud related to the charts entities to the MQTT broker. From there, the messages reach the relevant agent.
#### cloud-charts-service
Exposes API endpoints to fetch the chart metdata.
Forwards data requests via the `cloud-agent-data-ctrl-service` to the relevant agents to fetch chart data points. 
Exposes API endpoints to call various other endpoints on the agent, for instance functions.
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
Acts as a middleware for authentication on most of API endpoints. Validates incoming token headers, injects relevant headers and forwards the requests.
#### cloud-metrics-exporter
Exports various metrics from an on prem cloud-install. Uses the Prometheus metric exposition format.
#### cloud-netdata-assistant
Exposes API endpoints to fetch a human friendly explanation of various netdata configuration options, namely the alerts.
#### cloud-node-mqtt-input-service
Forwards MQTT messages emitted by the agent related to the node entities to the internal Pulsar broker. These include the node metadata as well as their connectivity state, either direct or via parents. 
#### cloud-node-mqtt-output-service
Forwards Pulsar messages emitted in the cloud related to the charts entities to the MQTT broker. From there, the messages reach the relevant agent.
#### cloud-notifications-dispatcher-service
Exposes API endpoints to handle integrations.
Handles incoming notification messages and uses the relevant channels(email, slack...) to notify relevant users.
#### cloud-spaceroom-service
Exposes API endpoints to fetch and store relations between agents, nodes, spaces, users and rooms.
Acts as a provider of authorization for other cloud endpoints.
Exposes API endpoints to authenticate agents connecting to the cloud.

## Infrastructure Diagram

![infrastructure.jpeg](infrastructure.jpeg)

## Basic troubleshooting
We cannot predict how your particular installation of Netdata Cloud On-prem is going to work. It is a mixture of underlying infrastructure, the number of agents, and their topology. You can always contact the Netdata team for recommendations!

#### Loading charts takes long time or ends with error
Charts service is trying to collect the data from all of the agents in question. If we are talking about the overview screen, all of the nodes in space are going to be queried (`All nodes` room). If it takes a long time, there are a few things that should be checked:
1. How many nodes are you querying directly?
  There is a big difference between having 100 nodes connected directly to the cloud compared to them being connected through a few parents. Netdata always prioritizes querying nodes through parents. This way, we can reduce some of the load by pushing the responsibility to query the data to the parent. The parent is then responsible for passing accumulated data from nodes connected to it to the cloud.
1. If you are missing data from endpoints all the time.
  Netdata Cloud always queries nodes themselves for the metrics. The cloud only holds information about metadata, such as information about what charts can be pulled from any node, but not the data points themselves for any metric. This means that if a node is throttled by the network connection or under high resource pressure, the information exchange between the agent and cloud through the MQTT broker might take a long time. In addition to checking resource usage and networking, we advise using a parent node for such endpoints. Parents can hold the data from nodes that are connected to the cloud through them, eliminating the need to query those endpoints.
1. Errors on the cloud when trying to load charts.
  If the entire data query is crashing and no data is displayed on the UI, it could indicate problems with the `cloud-charts-service`. It is possible that the query you are performing is simply exceeding the CPU and/or memory limits set on the deployment. We advise increasing those resources.

#### It takes long time to load anything on the Cloud UI
When experiencing sluggishness and slow responsiveness, the following factors should be checked regarding the Postgres database:
  1. CPU: Monitor the CPU usage to ensure it is not reaching its maximum capacity. High and sustained CPU usage can lead to sluggish performance.
  1. Memory: Check if the database server has sufficient memory allocated. Inadequate memory could cause excessive disk I/O and slow down the database.
  1. Disk Queue / IOPS: Analyze the disk queue length and disk I/O operations per second (IOPS). A high disk queue length or limited IOPS can indicate a bottleneck and negatively impact database performance.
By examining these factors and ensuring that CPU, memory, and disk IOPS are within acceptable ranges, you can mitigate potential performance issues with the Postgres database.

#### Nodes are not updated quickly on the Cloud UI
If youre experiencing delays with information exchange between the Cloud UI and the Agent, and youve already checked the networking and resource usage on the agent side, the problem may be related to Apache Pulsar or the database. Slow alerts on node alerts or slow updates on node status (online/offline) could indicate issues with message processing or database performance. You may want to investigate the performance of Apache Pulsar, ensure it is properly configured, and consider scaling or optimizing the database to handle the volume of data being processed or written to it.

### If you have any questions or suggestions please contact netdata team.