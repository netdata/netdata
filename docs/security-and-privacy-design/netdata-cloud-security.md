# Netdata Cloud Security and Privacy Design

Netdata Cloud is designed with a security-first approach to ensure the highest level of protection for user data. When
using Netdata Cloud in environments that require compliance with standards like PCI DSS, SOC 2, or HIPAA, users can be
confident that all collected data is stored within their infrastructure. Data viewed on dashboards and alert
notifications travel over Netdata Cloud, but aren’t stored—instead, they're transformed in transit, aggregated from
multiple Agents and parents (centralization points), to appear as one data source in the user's browser.

## User Identification and Authorization

Netdata Cloud requires only an email address to create an account and use the service. User identification and
authorization are conducted either via third-party integrations (Google, GitHub accounts) or through short-lived access
tokens sent to the user’s email account. Email addresses are stored securely in our production database on AWS and are
also used for product and marketing communications. Netdata Cloud doesn’t store user credentials.

## Data Storage and Transfer

Although Netdata Cloud doesn’t store metric data, it does keep some metadata for each node connected to user spaces.
This metadata includes the hostname, information from the `/api/v1/info` endpoint, metric metadata
from `/api/v1/contexts`, and alerts configurations from `/api/v1/alarms`. This data is securely stored in our production
database on AWS and copied to Google BigQuery for analytics purposes.

All data visible on Netdata Cloud is transferred through the Agent-Cloud link (ACLK) mechanism, which securely connects
a Netdata Agent to Netdata Cloud. The ACLK is encrypted and safe, and is only established if the user connects/claims
their node. Data in transit between a user and Netdata Cloud is encrypted using TLS.

## Data Retention and Erasure

Netdata Cloud retains deleted customer content for 90 days. Users can access, modify, and delete their personal data through self-service tools. If needed, users can request data deletion in writing, which Netdata will process in accordance with data protection laws.

## Infrastructure and Authentication

Netdata Cloud operates on an Infrastructure as Code (IaC) model. Its microservices environment is completely isolated,
and all changes occur through Terraform. At the edge of Netdata Cloud, there is a TLS termination and an Identity and
Access Management (IAM) service that validates JWT tokens included in request cookies.

Netdata Cloud does not store user credentials.

## Security Features and Response

Netdata Cloud offers a variety of security features, including infrastructure-level dashboards, centralized alert notifications, auditing logs, and role-based access to different segments of the infrastructure. It employs several protection mechanisms against DDoS attacks, such as rate-limiting and automated blocklisting. It also uses static code analyzers to prevent other types of attacks.

In the event of potential security vulnerabilities or incidents, Netdata Cloud follows the same process as the Netdata
agent. Every report is acknowledged and analyzed by the Netdata team within three working days, and the team keeps the
reporter updated throughout the process.

## User Customization

Netdata Cloud uses the highest level of security. There is no user customization available out of the box. Its security
settings are designed to provide maximum protection for all users. We are offering customization (like custom SSO
integrations, custom data retention policies, advanced user access controls, tailored audit logs, integration with other
security tools, etc.) on a per-contract basis.

## Deleting Personal Data

Users who wish to remove all personal data (including email and activities) can delete their account by logging into Netdata Cloud and accessing their profile.

## User Privacy and Data Protection

Netdata Cloud is built with an unwavering commitment to user privacy and data protection. We understand that our users'
data is both sensitive and valuable, and we’ve implemented stringent measures to ensure its safety.

### Data Collection

Netdata Cloud collects minimal personal information from its users. The only personal data required to create an account
and use the service is an email address. This email address is used for product and marketing communications.
Additionally, the IP address used to access Netdata Cloud is stored in web proxy access logs.

### Data Usage

The collected email addresses are stored in our production database on Amazon Web Services (AWS) and copied to Google
BigQuery, our data lake, for analytics purposes. These analytics are crucial for our product development process. If a
user accepts the use of analytical cookies, their email address and IP are stored in the systems we use to track
application usage (Google Analytics, Posthog, and Gainsight PX). Stripe handles subscriptions and Payments data.

### Data Sharing

Netdata Cloud does not share any personal data with third parties, ensuring the privacy of our users' data, but Netdata
Cloud does use third parties for its services, including, but not limited to, Google Cloud and Amazon Web Services for
its infrastructure, Stripe for payment processing, Google Analytics, Posthog and Gainsight PX for analytics.

### Data Protection

We use the newest security measures to protect user data from unauthorized access, use, or disclosure. All
infrastructure data visible on Netdata Cloud passes through the Agent-Cloud Link (ACLK) mechanism, which securely
connects a Netdata Agent to Netdata Cloud. The ACLK is encrypted, safe, and is only established if the user connects
their node. All data in transit between a user and Netdata Cloud is encrypted using TLS.

### User Control over Data

Netdata provides its users with the ability to access, retrieve, correct, and delete their personal data stored in Netdata Cloud.
This ability may occasionally be limited due to temporary service outages for maintenance or other updates to Netdata Cloud, or when it is technically not possible.
If self-service data deletion isn't possible, Netdata will process written deletion requests within DPA-specified timeframes, in compliance with data protection laws.

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
