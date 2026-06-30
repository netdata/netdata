# Enterprise SSO Authentication

Enterprise Single Sign-On (SSO) integration enables you to manage Netdata Cloud access through your existing identity management solution. This simplifies user authentication and improves security through centralized access control.

:::important

Enterprise SSO handles authentication only. You must configure user and role management separately within Netdata Cloud.

:::

## Prerequisites

| Requirement        | Details                                                                                                                                                         |
|--------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------|
| **SSO Provider**   | Must be [supported by Netdata](https://learn.netdata.cloud/docs/netdata-cloud/authentication-&-authorization/cloud-authentication-&-authorization-integrations) |
| **Account Status** | Active Netdata Cloud account                                                                                                                                    |
| **Subscription**   | Paid plan                                                                                                                                   |
| **Access Level**   | Space Administrator permissions                                                                                                                                 |

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

## Multi-Factor Authentication (MFA)

With Enterprise SSO, multi-factor authentication is handled entirely by your identity provider. Netdata Cloud does not store MFA credentials or run its own MFA challenge — it inherits the authentication policy enforced by your identity provider (for example, Okta, Azure AD, or Google Workspace).

Because the SSO flow redirects users to the identity provider for authentication, any MFA challenge, conditional access rule, or sign-on policy configured at the identity provider is applied before access to Netdata Cloud is granted. Netdata Cloud cannot bypass it.

To require MFA for all Netdata Cloud users in your organization:

1. Configure MFA as a sign-on policy in your identity provider.
2. Apply the policy to the application or user group associated with Netdata Cloud.

There is no separate MFA setting to enable inside Netdata Cloud — your identity provider is the single point of control.

:::note

See [Authentication & Authorization](/docs/netdata-cloud/authentication-and-authorization/README.md#multi-factor-authentication-mfa) for MFA availability by sign-in method.

:::
