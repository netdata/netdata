# Enterprise SSO Authentication

Enterprise Single Sign-On (SSO) integration enables organizations to manage Netdata Cloud access through their existing identity management solution. This simplifies user authentication and improves security through centralized access control.

> **Important**: Enterprise SSO handles authentication only. User and role management must be configured separately within Netdata Cloud.

## Prerequisites

| Requirement    | Details                                                                                                                                                         |
|----------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------|
| SSO Provider   | Must be [supported by Netdata](https://learn.netdata.cloud/docs/netdata-cloud/authentication-&-authorization/cloud-authentication-&-authorization-integrations) |
| Account Status | Active Netdata Cloud account                                                                                                                                    |
| Subscription   | Business plan or higher                                                                                                                                         |
| Access Level   | Space Administrator permissions                                                                                                                                 |

## Setup

**Netdata Cloud Configuration**:

To configure SSO in your Netdata Cloud space:

1. Navigate to Space Settings (gear icon above profile)
2. Select User Management → Authentication & Authorization
3. Locate your desired SSO integration
4. Click "Configure" and fill in the required integration attributes

**Domain Verification**:

Domain verification is required to establish secure SSO connectivity:

1. Access the DNS TXT record:
    - Go to Space Settings → User Management → Authentication & Authorization
    - Click "DNS TXT record" button to reveal verification code
2. Add DNS Record:
    - Log into your domain provider's DNS management
    - Create a new TXT record with these specifications:

      | Field                    | Value                                        |
      |--------------------------|----------------------------------------------|
      | Value/Answer/Description | `"netdata-verification=[VERIFICATION CODE]"` |
      | Name/Host/Alias          | Leave blank or use @ for subdomain           |
      | TTL (Time to Live)       | 86400 (or use provider default)              |

**SSO Provider Configuration**: Consult your provider's documentation for detailed instructions.

## How to Authenticate

Click on the link `Sign-in with an Enterprise Single Sign-On (SSO)` and follow the instructions. If you're not authenticated on the Enterprise SSO tool, you'll be prompted to authenticate there first before being allowed to proceed to Netdata Cloud.
