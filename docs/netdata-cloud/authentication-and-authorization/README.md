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

### Magic Link Authentication

When you sign in using your email address, Netdata Cloud sends a magic link to your inbox. This passwordless authentication method has the following security properties:

- **Expiration**: Magic links expire 15 minutes after generation
- **Single-use**: Magic links are automatically invalidated after successful authentication
- **Device flexibility**: If you open a magic link on one device, the link remains valid until used or expired, allowing you to complete the login on another device within the 15-minute window

## Authorization

Once logged in, you can manage role-based access in your Space to give each team member the appropriate role. For more information, see [Role-Based Access model](/docs/netdata-cloud/authentication-and-authorization/role-based-access-model.md).
