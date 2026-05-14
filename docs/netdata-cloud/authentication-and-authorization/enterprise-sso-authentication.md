# Enterprise SSO Authentication

Enterprise Single Sign-On (SSO) integration enables you to manage Netdata Cloud access through your existing identity management solution. This simplifies user authentication and improves security through centralized access control.

:::note

Enterprise SSO is available for both the Netdata Cloud Service (Business plan and above) and Enterprise On-Prem (self-hosted) deployments.

:::

:::important

Enterprise SSO handles authentication only. You must configure user and role management separately within Netdata Cloud.

:::

## Prerequisites

| Requirement        | Details                                                                                                                                                         |
|--------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------|
| **SSO Provider**   | Must be [supported by Netdata](https://learn.netdata.cloud/docs/netdata-cloud/authentication-&-authorization/cloud-authentication-&-authorization-integrations) |
| **Account Status** | Active Netdata Cloud account                                                                                                                                    |
| **Subscription**   | Business plan or higher, or Enterprise On-Prem                                                                                                                  |
| **Access Level**   | Space Administrator permissions                                                                                                                                 |

:::note

Enterprise On-Prem deployments support OIDC-based SSO integration with identity providers that implement the OpenID Connect protocol. Refer to your On-Prem deployment configuration for OIDC setup details.

:::
 
## Setup

### Netdata Cloud Configuration

To configure SSO in your Netdata Cloud space:

1. Navigate to **Space Settings** (⚙️) on the left sidebar below the spaces list
2. Select User Management → Authentication & Authorization
3. Locate your desired SSO integration
4. Click "Configure" and fill in the required integration attributes

### Domain Verification

Domain verification is required to establish secure SSO connectivity:

1. **Access the DNS TXT record:**
    - Go to Space Settings → User Management → Authentication & Authorization
    - Click "DNS TXT record" button to reveal verification code

2. **Add DNS Record:**
    - Log into your domain provider's DNS management
    - Create a new TXT record with these specifications:

| Field                        | Value                                        |
|------------------------------|----------------------------------------------|
| **Value/Answer/Description** | `"netdata-verification=[VERIFICATION CODE]"` |
| **Name/Host/Alias**          | Leave blank or use @ for subdomain           |
| **TTL (Time to Live)**       | 86400 (or use provider default)              |

### SSO Provider Configuration

Consult your provider's documentation for detailed instructions.

## How to Authenticate

Click on the link `Sign-in with an Enterprise Single Sign-On (SSO)` and follow the instructions. If you're not authenticated on the Enterprise SSO tool, you'll be prompted to authenticate there first before being allowed to proceed to Netdata Cloud.
