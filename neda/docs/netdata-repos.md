## Repository Intelligence Base

Netdata has 150+ active repos. Precise targeting is essential to avoid running out of turns or context window.

### Architecture of the Netdata ecosystem

- Users install Netdata Agents (netdata/netdata)
- Netdata Agents run Netdata UI (netdata/cloud-frontend)
- Netdata Agents connect to Netdata Cloud (netdata/cloud-*/)
- Netdata Cloud is Kubernetes microservice architecture
- Connection is Agent (via ACLK) -> Kubernetes -> EMQX -> cloud-*-bridge -> Pulsar -> cloud-*-service
- ACLK = MQTT over Websocket over HTTPS
- Netdata UI on Agent (http://host:19999) and Cloud (https://app.netdata.cloud) is the SAME (netdata/cloud-frontend)
- Cloud receives nodes, metric metadata (contexts) and alert transitions in real-time via ACLK
- Cloud provides multi-node, infrastructure level dashboards via netdata/cloud-charts-service
- cloud-charts-service queries agents in real-time, via ACLK
- Netdata UI (cloud-frontend) depends on netdata/netdata-ui and netdata/charts

### Netdata Agent
Core monitoring agent and deployment tools

- netdata/netdata: Netdata Agent source code, main support site (public)
  * /src: Core source code
    - /aclk: Agent Cloud Link communication
    - /collectors: Data collection plugins  
    - /database: Time-series database engine
    - /health: Alerting and health monitoring
    - /ml: Machine learning components
    - /web: Web server and API
    - /go: Go collectors and plugins
    - /libnetdata: Core libraries
    - /streaming: Data streaming between agents
    - /exporting: Export to external databases
    - /daemon: Core daemon functionality
    - /cli: Command line tools
    - /claim: Agent claiming to cloud
    - /registry: Node registry service
    - /crates: Rust components
    - /plugins.d: External plugin orchestration
  * /packaging: Installation and distribution
  * /integrations: Third-party integrations metadata
  * /docs: Documentation
  * /tests: Test suites
  * /system: System configuration files

- netdata/helmchart: Netdata Agent Helm chart for Kubernetes deployments (public)
- netdata/kernel-collector: eBPF collectors for Linux kernel metrics (public)
- netdata/ebpf-co-re: CO-RE eBPF programs for kernel monitoring (public)

### Netdata Cloud Backend
Cloud infrastructure and backend microservices (all actively maintained)

#### Core Backend Services
- netdata/cloud-spaceroom-service: Space and room management service (private)
- netdata/cloud-iam-user-service: Identity and access management service (private)
- netdata/cloud-accounts-service: User accounts management service (private)
- netdata/cloud-environment-service: Environment configuration service (private)

#### Data Pipeline Services
- netdata/cloud-agent-data-ctrl-service: Agent data controller, relays data requests (private)
- netdata/cloud-charts-service: Chart metadata storage and retrieval (private)
- netdata/cloud-custom-dashboard-service: Custom dashboard management (private)
- netdata/cloud-insights-service: AI investigations/troubleshooting/reporting service - with access to observability data (private)
- netdata/cloud-feed-service: Activity feed service (private)

#### MQTT/Messaging Services
- netdata/cloud-agent-mqtt-input-service: MQTT to Pulsar bridge for agent messages (private)
- netdata/cloud-node-mqtt-input-service: Node data MQTT input handler (private)
- netdata/cloud-node-mqtt-output-service: Node data MQTT output handler (private)
- netdata/cloud-charts-mqtt-input-service: Charts MQTT input handler (private)
- netdata/cloud-charts-mqtt-output-service: Charts MQTT output handler (private)
- netdata/cloud-mqtt-output-common: Common MQTT output utilities (private)

#### Alarm/Alert Services
- netdata/cloud-alarm-processor-service: Alarm processing engine (private)
- netdata/cloud-alarm-streaming-service: Real-time alarm streaming (private)
- netdata/cloud-alarm-config-mqtt-input-service: Alarm configuration via MQTT (private)
- netdata/cloud-alarm-log-mqtt-input-service: Alarm log MQTT handler (private)
- netdata/cloud-alarm-mqtt-output-service: Alarm MQTT output handler (private)
- netdata/cloud-notifications-dispatcher-service: Notification dispatch service (private)

#### Infrastructure & Operations
- netdata/cloud-metrics-exporter: Cloud metrics export service (private)
- netdata/cloud-telemetry-service: Telemetry collection service (private)
- netdata/cloud-bumper-service: Version bumping service (private)
- netdata/cloud-migrations: Database migration tools (private)
- netdata/cloud-workflows: Workflow automation (private)

#### Templates & Schemas
- netdata/cloud-schemas: Cloud data schemas (private)
- netdata/aclk-schemas: Agent-Cloud Link protobuf schemas (public)

#### Specialized Services
- netdata/cloud-licenser: License management service (private)
- netdata/cloud-admin-panel: Administrative panel (private)
- netdata/cloud-netdata-assistant: AI documentation assistant service - cannot troubleshoot - only for docs and config (private)
- netdata/cloud-insights-service: AI investigations/troubleshooting/reporting service - with access to observability data (private)
- netdata/cloud-api-docs: API documentation (private)
- netdata/cloud-kit: Cloud development kit (private)

### Netdata Cloud Frontend
UI components and visualization

- netdata/cloud-frontend: Netdata Cloud UI source code (private)
- netdata/netdata-ui: UI components library and toolkit (public)
- netdata/charts: Frontend charting SDK and visualization library (public)

Users can download the latest compiled Netdata Cloud UI (or Netdata UI for short, commonly referred as Netdata Dashboard too) compiled. No source code is publicly available. The Netdata Agent installer downloads and installs the latest version of the UI and it is also publicly available via cloudflare.

### AI Assistants

All in-app AI features which require observability data (troubleshooting, root cause analysis, alert analysis, reporting, etc) are handled by the cloud-insights-service:

- netdata/cloud-insights-service: AI investigations/troubleshooting/reporting service - with access to observability data (private)

All documentation and configuration requests are handled by the cloud-netdata-assistant (this service does not have access to observability data):

- netdata/cloud-netdata-assistant: Cloud AI documentation assistant backend - cannot troubleshoot - only for docs and config (private)

Publicly available documentation assistant (this appears at the home page of learn.netdata.cloud):

- netdata/ask-netdata: AI documentation assistant integrated in learn.netdata.cloud (public)

Our CRM agent used internally in slack:

- netdata/ai-agent: AI agent framework powering Neda assistant (public)

### DevOps & Infrastructure
Infrastructure automation and deployment

- netdata/infra: Main infrastructure as code repository (private)
- netdata/ansible: Ansible playbooks for Netdata deployment (public)
- netdata/terraform-provider-netdata: Terraform provider for Netdata Cloud (public)
- netdata/helper-images: Docker base and builder images (public)
- netdata/golang-base: Go base images and utilities (public)

#### Infrastructure Modules
- netdata/infra-terraform-module-template: Terraform module template (private)
- netdata/infra-terraform-module-vpc: VPC configuration module (private)
- netdata/infra-terraform-module-eks: EKS cluster module (private)
- netdata/infra-terraform-module-eks-resources: EKS resources module (private)
- netdata/infra-terraform-module-ec2: EC2 instance module (private)
- netdata/infra-terraform-module-ecr: Container registry module (private)
- netdata/infra-terraform-module-rds: RDS database module (private)
- netdata/infra-terraform-module-elasticache: ElastiCache module (private)
- netdata/infra-terraform-module-elastic: Elasticsearch module (private)
- netdata/infra-terraform-module-s3: S3 bucket module (private)
- netdata/infra-terraform-module-s3-public: Public S3 bucket module (private)
- netdata/infra-terraform-module-lambda: Lambda function module (private)
- netdata/infra-terraform-module-iam: IAM roles and policies module (private)
- netdata/infra-terraform-module-security: Security configurations module (private)
- netdata/infra-terraform-module-cloudflare: Cloudflare configuration (private)
- netdata/infra-terraform-module-datadog: Datadog monitoring module (private)
- netdata/infra-terraform-module-pagerduty: PagerDuty integration module (private)
- netdata/infra-terraform-module-billing: Billing management module (private)
- netdata/infra-terraform-module-aws-backup: AWS backup configuration (private)
- netdata/infra-terraform-module-github-oidc: GitHub OIDC module (private)
- netdata/infra-terraform-module-github-repositories: GitHub repo management (private)
- netdata/infra-terraform-module-ps: Parameter store module (private)

#### Infrastructure Tools
- netdata/infra-helmcharts: Helm charts for infrastructure (private)
- netdata/team-sre-shared-creds: Shared credentials management (private)

### Websites & Documentation
Public-facing websites and documentation

- netdata/website: Marketing website https://www.netdata.cloud (private)
- netdata/learn: Documentation website https://learn.netdata.cloud (public)
- netdata/community: Community examples and applications (public)

### Support & Issues
Public support and issue tracking

- netdata/netdata-cloud: Tickets, bugs, feature requests for Netdata Cloud (public)
- netdata/netdata-cloud-onprem: On-premises cloud support (public)
- netdata/.github: Organization-wide community health files (public)

### Integrations & Extensions
Third-party integrations and data tools

- netdata/netdata-grafana-datasource-plugin: Grafana data source plugin (public)
- netdata/netdata-grafana-datasource-plugin-test-workflows: Grafana plugin tests (public)

### Analytics & Business Intelligence
Usage tracking and analytics

- netdata/analytics-bi: Business intelligence pipelines (private)
- netdata/marketing: Marketing operations (private)

### Mobile & Desktop Apps
Native applications

- netdata/netdata-mobile-app-android: Android mobile application (public)
- netdata/netdata-mobile-app-ios: iOS mobile application (public)

### Quality Assurance

- netdata/privacy_check: Privacy compliance verification for standalone Netdata agents (public)

### Internal Tools & Resources
Internal company tools

- netdata/internal: Internal tools and resources (private)
- netdata/demo-env: Demo environment setup (public)

### Development Tools & Libraries
Shared libraries and utilities

- netdata/feed-ontology: Feed data ontology (public)
- netdata/opap: Object property access protocol (public)

### Security Advisories
Security vulnerability tracking (private)

- netdata/netdata-ghsa-xfp2-8264-4w82: Security advisory (private)  
- netdata/netdata-ghsa-xhw4-q8f5-2j4c: Security advisory (private)


## Best Practices For Searching

1. Netdata Agents (children) communicate to Netdata Parents via streaming. This is a single bidirectional socket, initiated from the child to the parent, although there is a small handshake before this is established, using simple http(s) calls, in order to support load balancing parents.
2. Netdata Agents and Parents communicate to Netdata Cloud via MQTT over WebSocket over HTTPS. There is a small handshake before this is established, for authorization and settings.
3. Vnode = virtual node. This has nothing to do with virtualization or containerization. It is used by Netdata to create node entities for remotely monitored systems and applications, allowing users to see on the dashboard cloud provider managed databases, SNMP devices, or even systems like IBM i. vnodes are mostly implemented by go.d.plugin.
4. DYNCFG = Netdata's UI based configuration. This exists mostly for alerts and golang/rust based plugins, allowing them to be configured on the fly.
5. DYNCFG is able to configure any Netdata Agent in a Netdata ecosystem, as long as it is somehow (directly or indirectly) connected to Netdata Cloud. The streaming protocol of Netdata enables this, by routing DYNCFG requests and responses to Netdata Agents via their parents.
6. DYNCFG is the `config` API endpoint (GET/POST).
7. Dashboard = cloud-frontend, which is the same for both Netdata Agents/Parents and Netdata Cloud.

IMPORTANT: Netdata uses unusual API endpoints and searching for POST/PUT/GET on assumed/expected endpoints may reveal nothing. Before concluding that some API functionality is not supported, you should first trace the functionality backwards: find the code that implements the features, find how it is exposed in the APIs, find which components use these APIs, examine what features these components offer.

IMPORTANT: When searching in the repos, do not make case sensitive searches, unless you know you are looking for something specific that is case sensitive. Do not assume snake case, camel case, etc. Different teams (agent, front-end, cloud backend, SREs) may use different formats.
