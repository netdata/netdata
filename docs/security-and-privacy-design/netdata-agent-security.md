# Netdata Agent Security and Privacy Design

## Security by Design

Netdata Agent is designed with a security-first approach. Its structure ensures data safety by only exposing chart
metadata and metric values, not the raw data collected. This design principle allows Netdata to be used in environments
requiring the highest level of data isolation, such as PCI Level 1. Even though Netdata plugins connect to a user's
database server or read application log files to collect raw data, only the processed metrics are stored in Netdata
databases, sent to upstream Netdata servers, or archived to external time-series databases.

## User Data Protection

> **Note**
>
> Users are responsible for backing up, recovering, and ensuring their data's availability because Netdata stores data locally on each system due to its decentralized architecture.

The Netdata Agent is programmed to safeguard user data. When collecting data, the raw data does not leave the host. All
plugins, even those running with escalated capabilities or privileges, perform a hard-coded data collection job. They do
not accept commands from Netdata, and the original application data collected do not leave the process they are
collected in, are not saved, and are not transferred to the Netdata daemon. For the “Functions” feature, the data
collection plugins offer Functions, and the user interface merely calls them back as defined by the data collector. The
Netdata Agent main process does not require any escalated capabilities or privileges from the operating system, and
neither do most of the data collecting plugins.

## Communication and Data Encryption

Data collection plugins communicate with the main Netdata process via ephemeral, in-memory, pipes that are inaccessible
to any other process.

Streaming of metrics between Netdata Agents requires an API key and can also be encrypted with TLS if the user
configures it.

The Netdata Agent's web API can also use TLS if configured.

When Netdata Agents are connected to the Cloud, the communication happens via MQTT over Web Sockets over TLS, and
public/private keys are used for authorizing access. These keys are exchanged during the connecting process (usually
during the provisioning of each Agent).

## Authentication

Direct user access to the Agent is not authenticated, considering that users should either use Netdata Cloud, or they
are already on the same LAN, or they have configured proper firewall policies. However, Netdata Agents can be hidden
behind an authenticating web proxy if required.

For other Netdata Agents streaming metrics to an Agent, authentication via API keys is required and TLS can be used if
configured.

For Netdata Cloud accessing Netdata Agents, public/private key cryptography is used and TLS is mandatory.

## Security Vulnerability Response

If a security vulnerability is found in the Netdata Agent, the Netdata team acknowledges and analyzes each report within
three working days, kicking off a Security Release Process. Any vulnerability information shared with the Netdata team
stays within the Netdata project and is not disseminated to other projects unless necessary for fixing the issue. The
reporter is kept updated as the security issue moves from triage to identified fix, to release planning. More
information can be found [here](https://github.com/netdata/netdata/security/policy).

## Protection Against Common Security Threats

The Netdata Agent is resilient against common security threats such as DDoS attacks and SQL injections. For DDoS, the Agent uses a fixed number of threads for processing requests, providing a cap on the resources that can be
consumed. It also automatically manages its memory to prevent over-utilization. SQL injections are prevented as nothing
from the UI is passed back to the data collection plugins accessing databases.

Additionally, the Agent is running as a normal, unprivileged, operating system user (a few data collections
require escalated privileges, but these privileges are isolated to just them), every netdata process runs by default
with a nice priority to protect production applications in case the system is starving for CPU resources, and Netdata
agents are configured by default to be the first processes to be killed by the operating system in case the operating
system starves for memory resources (OS-OOM - Operating System Out Of Memory events).

## User-Customizable Security Settings

Netdata provides users with the flexibility to customize the Agent's security settings. Users can configure TLS across the system, and the Agent provides extensive access control lists on all its interfaces to limit access to its endpoints based on IP. Additionally, users can configure the CPU and Memory priority of Netdata Agents.
