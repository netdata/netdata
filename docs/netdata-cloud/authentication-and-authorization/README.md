# Authentication & Authorization

Learn how to authenticate with Netdata Cloud and manage team member permissions through role-based authorization.

## Authentication

| Method             | Description                                                                                                                                                                                                                   | Setup Process                                                                                                              |
|:-------------------|:------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|:---------------------------------------------------------------------------------------------------------------------------|
| **Email**          | • Standard email and password authentication<br/>• Recommended for individual users                                                                                                                                           | 1. Visit Netdata Cloud<br/>2. Enter email address<br/>3. Follow verification process<br/>4. Set up password (new accounts) |
| **Google OAuth**   | • Authentication using Google account credentials<br/>• Your account will be linked to your Google email address                                                                                                              | 1. Visit Netdata Cloud<br/>2. Click Google sign-in<br/>3. Complete Google authentication flow                              |
| **GitHub OAuth**   | • Authentication using GitHub account credentials<br/>• Your account will be linked to your GitHub email address                                                                                                              | 1. Visit Netdata Cloud<br/>2. Click GitHub sign-in<br/>3. Complete GitHub authentication flow                              |
| **Enterprise SSO** | • Advanced authentication for organizations using identity providers<br/>• Features:<br/>&emsp; - Identity provider integration<br/>&emsp; - Centralized management<br/>&emsp; - Enhanced security<br/>&emsp; - Audit logging | See [Enterprise SSO documentation](/docs/netdata-cloud/authentication-and-authorization/enterprise-sso-authentication.md)  |

:::important

When using OAuth, your Netdata Cloud account will be automatically associated with the email address provided by the OAuth provider. Ensure you have access to this email address.

:::

## Multi-Factor Authentication (MFA)

Multi-factor authentication (MFA) adds an extra layer of protection to your Netdata Cloud account. The availability of MFA depends on which sign-in method you use.

| Method             | MFA Availability                                                                                                                     |
|:-------------------|:-------------------------------------------------------------------------------------------------------------------------------------|
| **Email**          | Netdata Cloud does not currently offer built-in MFA for email/password accounts.                                                     |
| **Google OAuth**   | MFA is managed by Google. Enable it in your [Google Account security settings](https://myaccount.google.com/security).               |
| **GitHub OAuth**   | MFA is managed by GitHub. Enable it in your [GitHub account settings](https://github.com/settings/security).                         |
| **Enterprise SSO** | MFA is configured and enforced by your identity provider (e.g., Okta, Azure AD, Google Workspace) during the SSO authentication flow. |

For organizations that require MFA for all users, [Enterprise SSO](/docs/netdata-cloud/authentication-and-authorization/enterprise-sso-authentication.md) delegates authentication entirely to your identity provider, which can enforce MFA policies centrally.

## Authorization

Once logged in, you can manage role-based access in your Space to give each team member the appropriate role. For more information, see [Role-Based Access model](/docs/netdata-cloud/authentication-and-authorization/role-based-access-model.md).
