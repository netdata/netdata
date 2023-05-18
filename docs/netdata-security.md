# Security and privacy design

This document serves as the relevant Annex to the [Terms of Service](https://www.netdata.cloud/service-terms/),
the [Privacy Policy](https://www.netdata.cloud/privacy/) and
the Data Processing Addendum, when applicable. It provides more information regarding Netdata’s technical and
organizational security and privacy measures.

We have given special attention to all aspects of Netdata, ensuring that everything throughout its operation is as
secure as possible. Netdata has been designed with security in mind.

## Netdata's Security Principles

### Security by Design

Netdata, an open-source software widely installed across the globe, prioritizes security by design, showcasing our
commitment to safeguarding user data. The entire structure and internal architecture of the software is built to ensure
maximum security. We aim to provide a secure environment from the ground up, rather than as an afterthought.

### Compliance with Open Source Security Foundation Best Practices

Netdata is committed to adhering to the best practices laid out by the Open Source Security Foundation (OSSF).
Currently, the Netdata Agent follows the OSSF best practices at the passing level. Feel free to audit our approach to 
the [OSSF guidelines](https://bestpractices.coreinfrastructure.org/en/projects/2231)

Netdata Cloud boasts of comprehensive end-to-end automated testing, encompassing the UI, back-end, and agents, where
involved. In addition, the Netdata Agent uses an array of third-party services for static code analysis, static code
security analysis, and CI/CD integrations to ensure code quality on a per pull request basis. Tools like Github's
CodeQL, Github's Dependabot, our own unit tests, various types of linters,
and [Coverity](https://scan.coverity.com/projects/netdata-netdata?tab=overview) are utilized to this end.

Moreover, each PR requires two code reviews from our senior engineers before being merged. We also maintain two
high-performance environments (a production-like kubernetes cluster and a highly demanding stress lab) for
stress-testing our entire solution. This robust pipeline ensures the delivery of high-quality software consistently.

### Regular Third-Party Testing and Isolation

While Netdata doesn't have a dedicated internal security team, the open-source Netdata Agent undergoes regular testing
by third parties. Any security reports received are addressed immediately. In contrast, Netdata Cloud operates in a
fully automated and isolated environment with Infrastructure as Code (IaC), ensuring no direct access to production
applications. Monitoring and reporting is also fully automated.

### Security Vulnerability Response

Netdata has a transparent and structured process for handling security vulnerabilities. We appreciate and value the
contributions of security researchers and users who report vulnerabilities to us. All reports are thoroughly
investigated, and any identified vulnerabilities trigger a Security Release Process.

We aim to fully disclose any bugs as soon as a user mitigation is available, typically within a week of the report. In
case of security fixes, we promptly release a new version of the software. Users can subscribe to our releases on GitHub
to stay updated about all security incidents. More details about our vulnerability response process can be
found [here](https://github.com/netdata/netdata/security/policy).

### Adherence to Open Source Security Foundation Best Practices

In line with our commitment to security, we uphold the best practices as outlined by the Open Source Security
Foundation. This commitment reflects in every aspect of our operations, from the design phase to the release process,
ensuring the delivery of a secure and reliable product to our users. For more information
check [here](https://bestpractices.coreinfrastructure.org/en/projects/2231).

## Netdata Agent Security

### Security by Design

Netdata Agent is designed with a security-first approach. Its structure ensures data safety by only exposing chart
metadata and metric values, not the raw data collected. This design principle allows Netdata to be used in environments
requiring the highest level of data isolation, such as PCI Level 1. Even though Netdata plugins connect to a user's
database server or read application log files to collect raw data, only the processed metrics are stored in Netdata
databases, sent to upstream Netdata servers, or archived to external time-series databases.

### User Data Protection

The Netdata Agent is programmed to safeguard user data. When collecting data, the raw data does not leave the host. All
plugins, even those running with escalated capabilities or privileges, perform a hard-coded data collection job. They do
not accept commands from Netdata, and the original application data collected do not leave the process they are
collected in, are not saved, and are not transferred to the Netdata daemon. For the “Functions” feature, the data
collection plugins offer Functions, and the user interface merely calls them back as defined by the data collector. The
Netdata Agent main process does not require any escalated capabilities or privileges from the operating system, and
neither do most of the data collecting plugins.

### Communication and Data Encryption

Data collection plugins communicate with the main Netdata process via ephemeral, in-memory, pipes that are inaccessible
to any other process.

Streaming of metrics between Netdata agents requires an API key and can also be encrypted with TLS if the user
configures it.

The Netdata agent's web API can also use TLS if configured.

When Netdata agents are claimed to Netdata Cloud, the communication happens via MQTT over Web Sockets over TLS, and
public/private keys are used for authorizing access. These keys are exchanged during the claiming process (usually
during the provisioning of each agent).

### Authentication

Direct user access to the agent is not authenticated, considering that users should either use Netdata Cloud, or they
are already on the same LAN, or they have configured proper firewall policies. However, Netdata agents can be hidden
behind an authenticating web proxy if required.

For other Netdata agents streaming metrics to an agent, authentication via API keys is required and TLS can be used if
configured.

For Netdata Cloud accessing Netdata agents, public/private key cryptography is used and TLS is mandatory.

### Security Vulnerability Response

If a security vulnerability is found in the Netdata Agent, the Netdata team acknowledges and analyzes each report within
three working days, kicking off a Security Release Process. Any vulnerability information shared with the Netdata team
stays within the Netdata project and is not disseminated to other projects unless necessary for fixing the issue. The
reporter is kept updated as the security issue moves from triage to identified fix, to release planning. More
information can be found [here](https://github.com/netdata/netdata/security/policy).

### Protection Against Common Security Threats

The Netdata agent is resilient against common security threats such as DDoS attacks and SQL injections. For DDoS,
Netdata agent uses a fixed number of threads for processing requests, providing a cap on the resources that can be
consumed. It also automatically manages its memory to prevent overutilization. SQL injections are prevented as nothing
from the UI is passed back to the data collection plugins accessing databases.

Additionally, the Netdata agent is running as a normal, unprivileged, operating system user (a few data collections
require escalated privileges, but these privileges are isolated to just them), every netdata process runs by default
with a nice priority to protect production applications in case the system is starving for CPU resources, and Netdata
agents are configured by default to be the first processes to be killed by the operating system in case the operating
system starves for memory resources (OS-OOM - Operating System Out Of Memory events).

### User Customizable Security Settings

Netdata provides users with the flexibility to customize agent security settings. Users can configure TLS across the
system, and the agent provides extensive access control lists on all its interfaces to limit access to its endpoints
based on IP. Additionally, users can configure the CPU and Memory priority of Netdata agents.

## Netdata Cloud Security

Netdata Cloud is designed with a security-first approach to ensure the highest level of protection for user data. When
using Netdata Cloud in environments that require compliance with standards like PCI DSS, SOC 2, or HIPAA, users can be
confident that all collected data is stored within their infrastructure. Data viewed on dashboards and alert
notifications travel over Netdata Cloud, but are not stored—instead, they're transformed in transit, aggregated from
multiple agents and parents (centralization points), to appear as one data source in the user's browser.

### User Identification and Authorization

Netdata Cloud requires only an email address to create an account and use the service. User identification and
authorization are conducted either via third-party integrations (Google, GitHub accounts) or through short-lived access
tokens sent to the user’s email account. Email addresses are stored securely in our production database on AWS and are
also used for product and marketing communications. Netdata Cloud does not store user credentials.

### Data Storage and Transfer

Although Netdata Cloud does not store metric data, it does keep some metadata for each node connected to user spaces.
This metadata includes the hostname, information from the `/api/v1/info` endpoint, metric metadata
from `/api/v1/contexts`, and alerts configurations from `/api/v1/alarms`. This data is securely stored in our production
database on AWS and copied to Google BigQuery for analytics purposes.

All data visible on Netdata Cloud is transferred through the Agent-Cloud link (ACLK) mechanism, which securely connects
a Netdata Agent to Netdata Cloud. The ACLK is encrypted and safe, and is only established if the user connects/claims
their node. Data in transit between a user and Netdata Cloud is encrypted using TLS.

### Data Retention and Erasure

Netdata Cloud maintains backups of customer content for approximately 90 days following a deletion. Users have the
ability to access, retrieve, correct, and delete personal data stored in Netdata Cloud. In case a user is unable to
delete personal data via self-services functionality, Netdata will delete personal data upon the customer's written
request, in accordance with applicable data protection law.

### Infrastructure and Authentication

Netdata Cloud operates on an Infrastructure as Code (IaC) model. Its microservices environment is completely isolated,
and all changes occur through Terraform. At the edge of Netdata Cloud, there is a TLS termination and an Identity and
Access Management (IAM) service that validates JWT tokens included in request cookies.

Netdata Cloud does not store user credentials.

### Security Features and Response

Netdata Cloud offers a variety of security features, including infrastructure-level dashboards, centralized alerts
notifications, auditing logs, and role-based access to different segments of the infrastructure. The cloud service
employs several protection mechanisms against DDoS attacks, such as rate-limiting and automated blacklisting. It also
uses static code analysers to prevent other types of attacks.

In the event of potential security vulnerabilities or incidents, Netdata Cloud follows the same process as the Netdata
agent. Every report is acknowledged and analyzed by the Netdata team within three working days, and the team keeps the
reporter updated throughout the process.

### User Customization

Netdata Cloud uses the highest level of security. There is no user customization available out of the box. Its security
settings are designed to provide maximum protection for all users. We are offering customization (like custom SSO
integrations, custom data retention policies, advanced user access controls, tailored audit logs, integration with other
security tools, etc.) on a per contract basis.

### Deleting Personal Data

Users who wish to remove all personal data (including email and activities) can delete their cloud account by logging
into Netdata Cloud and accessing their profile.

## User Privacy and Data Protection

Netdata Cloud is built with an unwavering commitment to user privacy and data protection. We understand that our users'
data is both sensitive and valuable, and we have implemented stringent measures to ensure its safety.

### Data Collection

Netdata Cloud collects minimal personal information from its users. The only personal data required to create an account
and use the service is an email address. This email address is used for product and marketing communications.
Additionally, the IP address used to access Netdata Cloud is stored in web proxy access logs.

### Data Usage

The collected email addresses are stored in our production database on Amazon Web Services (AWS) and copied to Google
BigQuery, our data lake, for analytics purposes. These analytics are crucial for our product development process. If a
user accepts the use of analytical cookies, their email address and IP are stored in the systems we use to track
application usage (Google Analytics, Posthog, and Gainsight PX). Subscriptions and Payments data are handled by Stripe.

### Data Sharing

Netdata Cloud does not share any personal data with third parties, ensuring the privacy of our users' data, but Netdata
Cloud does use third parties for its services, including, but not limited to, Google Cloud and Amazon Web Services for
its infrastructure, Stripe for payment processing, Google Analytics, Posthog and Gainsight PX for analytics.

### Data Protection

We use state-of-the-art security measures to protect user data from unauthorized access, use, or disclosure. All
infrastructure data visible on Netdata Cloud passes through the Agent-Cloud Link (ACLK) mechanism, which securely
connects a Netdata Agent to Netdata Cloud. The ACLK is encrypted, safe, and is only established if the user connects
their node. All data in transit between a user and Netdata Cloud is encrypted using TLS.

### User Control over Data

Netdata provides its users with the ability to access, retrieve, correct, and delete their personal data stored in
Netdata Cloud. This ability may occasionally be limited due to temporary service outages for maintenance or other
updates to Netdata Cloud, or when it is technically not feasible. If a customer is unable to delete personal data via
the self-services functionality, Netdata deletes the data upon the customer's written request, within the timeframe
specified in the Data Protection Agreement (DPA), and in accordance with applicable data protection laws.

### Compliance with Data Protection Laws

Netdata Cloud is fully compliant with data protection laws like the General Data Protection Regulation (GDPR) and the
California Consumer Privacy Act (CCPA).

### Data Transfer

Data transfer within Netdata Cloud is secure and respects the privacy of the user data. The Netdata Agent establishes an
outgoing secure WebSocket (WSS) connection to Netdata Cloud, ensuring that the data is encrypted when in transit.

### Use of Tracking Technologies

Netdata Cloud uses analytical cookies if a user consents to their use. These cookies are used to track the usage of the
application and are stored in systems like Google Analytics, Posthog and Gainsight PX.

### Data Breach Notification Process

In the event of a data breach, Netdata has a well-defined process in place for notifying users. The details of this
process align with the standard procedures and timelines defined in the Data Protection Agreement (DPA).

We continually review and update our privacy and data protection practices to ensure the highest level of data safety
and privacy for our users.

## Compliance with Regulations

Netdata is committed to ensuring the security, privacy, and integrity of user data. It complies with both the General
Data Protection Regulation (GDPR), a regulation in EU law on data protection and privacy, and the California Consumer
Privacy Act (CCPA), a state statute intended to enhance privacy rights and consumer protection for residents of
California.

### Compliance with GDPR and CCPA

Compliance with GDPR and CCPA are self-assessment processes, and Netdata has undertaken thorough internal audits and
controls to ensure it meets all requirements.

As per request basis, any customer may enter with Netdata into a data processing addendum (DPA) governing customer’s
ability to load and permit Netdata to process any personal data or information regulated under applicable data
protection laws, including the GDPR and CCPA.

### Data Transfers

While Netdata Agent itself does not engage in any cross-border data transfers, certain personal and infrastructure data
is transferred to Netdata Cloud for the purpose of providing its services. The metric data collected and processed by
Netdata Agents, however, stays strictly within the user's infrastructure, eliminating any concerns about cross-border
data transfer issues.

When users utilize Netdata Cloud, the metric data is streamed directly from the Netdata Agent to the users’ web browsers
via Netdata Cloud, without being stored on Netdata Cloud's servers. However, user identification data (such as email
addresses) and infrastructure metadata necessary for Netdata Cloud's operation are stored in data centers in the United
States, using compliant infrastructure providers such as Google Cloud and Amazon Web Services. These transfers and
storage are carried out in full compliance with applicable data protection laws, including GDPR and CCPA.

### Privacy Rights

Netdata ensures user privacy rights as mandated by the GDPR and CCPA. This includes the right to access, correct, and
delete personal data. These functions are all available online via the Netdata Cloud User Interface (UI). In case a user
wants to remove all personal information (email and activities), they can delete their cloud account by logging
into https://app.netdata.cloud and accessing their profile, at the bottom left of the screen.

### Regular Review and Updates

Netdata is dedicated to keeping its practices up-to-date with the latest developments in data protection regulations.
Therefore, as soon as updates or changes are made to these regulations, Netdata reviews and updates its policies and
practices accordingly to ensure continual compliance.

While Netdata is confident in its compliance with GDPR and CCPA, users are encouraged to review Netdata's privacy policy
and reach out with any questions or concerns they may have about data protection and privacy.

## Anonymous Statistics

The anonymous statistics collected by the Netdata Agent are related to the installations and not to individual users.
This data includes community size, types of plugins used, possible crashes, operating systems installed, and the use of
the registry feature. No IP addresses are collected, but each Netdata installation has a unique ID.

Netdata also collects anonymous telemetry events, which provide information on the usage of various features, errors,
and performance metrics. This data is used to understand how the software is being used and to identify areas for
improvement.

The purpose of collecting these statistics and telemetry data is to guide the development of the open-source agent,
focusing on areas that are most beneficial to users.

Users have the option to opt out of this data collection during the installation of the agent, or at any time by
removing a specific file from their system.

Netdata retains this data indefinitely in order to track changes and trends within the community over time.

Netdata does not share these anonymous statistics or telemetry data with any third parties.

By collecting this data, Netdata is able to continuously improve their service and identify any issues or areas for
improvement, while respecting user privacy and maintaining transparency.

## Internal Security Measures

Internal Security Measures at Netdata are designed with an emphasis on data privacy and protection. The measures
include:

1. **Infrastructure as Code (IaC)** :
   Netdata Cloud follows the IaC model, which means it is a microservices environment that is completely isolated. All
   changes are managed through Terraform, an open-source IaC software tool that provides a consistent CLI workflow for
   managing cloud services.
2. **TLS Termination and IAM Service** :
   At the edge of Netdata Cloud, there is a TLS termination, which provides the decryption point for incoming TLS
   connections. Additionally, an Identity Access Management (IAM) service validates JWT tokens included in request
   cookies or denies access to them.
3. **Session Identification** :
   Once inside the microservices environment, all requests are associated with session IDs that identify the user making
   the request. This approach provides additional layers of security and traceability.
4. **Data Storage** :
   Data is stored in various NoSQL and SQL databases and message brokers. The entire environment is fully isolated,
   providing a secure space for data management.
5. **Authentication** :
   Netdata Cloud does not store credentials. It offers three types of authentication: GitHub Single Sign-On (SSO),
   Google SSO, and email validation.
6. **DDoS Protection** :
   Netdata Cloud has multiple protection mechanisms against Distributed Denial of Service (DDoS) attacks, including
   rate-limiting and automated blacklisting.
7. **Security-Focused Development Process** :
   To ensure a secure environment, Netdata employs a security-focused development process. This includes the use of
   static code analysers to identify potential security vulnerabilities in the codebase.
8. **High Security Standards** :
   Netdata Cloud maintains high security standards and can provide additional customization on a per contract basis.
9. **Employee Security Practices** :
   Netdata ensures its employees follow security best practices, including role-based access, periodic access review,
   and multi-factor authentication. This helps to minimize the risk of unauthorized access to sensitive data.
10. **Experienced Developers** :
    Netdata hires senior developers with vast experience in security-related matters. It enforces two code reviews for
    every Pull Request (PR), ensuring that any potential issues are identified and addressed promptly.
11. **DevOps Methodologies** :
    Netdata's DevOps methodologies use the highest standards in access control in all places, utilizing the best
    practices available.
12. **Risk-Based Security Program** :
    Netdata has a risk-based security program that continually assesses and mitigates risks associated with data
    security. This program helps maintain a secure environment for user data.

These security measures ensure that Netdata Cloud is a secure environment for users to monitor and troubleshoot their
systems. The company remains committed to continuously improving its security practices to safeguard user data
effectively.

## PCI DSS

PCI DSS (Payment Card Industry Data Security Standard) is a set of security standards designed to ensure that all
companies that accept, process, store or transmit credit card information maintain a secure environment.

Netdata is committed to providing secure and privacy-respecting services, and it aligns its practices with many of the
key principles of the PCI DSS. However, it's important to clarify that Netdata is not officially certified as PCI
DSS-compliant. While Netdata follows practices that align with PCI DSS's key principles, the company itself has not
undergone the formal certification process for PCI DSS compliance.

PCI DSS compliance is not just about the technical controls but also involves a range of administrative and procedural
safeguards that go beyond the scope of Netdata's services. These include, among other things, maintaining a secure
network, implementing strong access control measures, regularly monitoring and testing networks, and maintaining an
information security policy.

Therefore, while Netdata can support entities with their data security needs in relation to PCI DSS, it is ultimately
the responsibility of the entity to ensure full PCI DSS compliance across all of their operations. Entities should
always consult with a legal expert or a PCI DSS compliance consultant to ensure that their use of any product, including
Netdata, aligns with PCI DSS regulations.

## HIPAA

HIPAA stands for the Health Insurance Portability and Accountability Act, which is a United States federal law enacted
in 1996. HIPAA is primarily focused on protecting the privacy and security of individuals' health information.

Netdata is committed to providing secure and privacy-respecting services, and it aligns its practices with many key
principles of HIPAA. However, it's important to clarify that Netdata is not officially certified as HIPAA-compliant.
While Netdata follows practices that align with HIPAA's key principles, the company itself has not undergone the formal
certification process for HIPAA compliance.

HIPAA compliance is not just about technical controls but also involves a range of administrative and procedural
safeguards that go beyond the scope of Netdata's services. These include, among other things, employee training,
physical security, and contingency planning.

Therefore, while Netdata can support HIPAA-regulated entities with their data security needs and is prepared to sign a
Business Associate Agreement (BAA), it is ultimately the responsibility of the healthcare entity to ensure full HIPAA
compliance across all of their operations. Entities should always consult with a legal expert or a HIPAA compliance
consultant to ensure that their use of any product, including Netdata, aligns with HIPAA regulations.

## Conclusion

In conclusion, Netdata Cloud's commitment to data security and user privacy is paramount. From the careful design of the
infrastructure and stringent internal security measures to compliance with international regulations and standards like
GDPR and CCPA, Netdata Cloud ensures a secure environment for users to monitor and troubleshoot their systems.

The use of advanced encryption techniques, role-based access control, and robust authentication methods further
strengthen the security of user data. Netdata Cloud also maintains transparency in its data handling practices, giving
users control over their data and the ability to easily access, retrieve, correct, and delete their personal data.

Netdata's approach to anonymous statistics collection respects user privacy while enabling the company to improve its
product based on real-world usage data. Even in such cases, users have the choice to opt-out, underlining Netdata's
respect for user autonomy.

In summary, Netdata Cloud offers a highly secure, user-centric environment for system monitoring and troubleshooting.
The company's emphasis on continuous security improvement and commitment to user privacy make it a trusted choice in the
data monitoring landscape.

