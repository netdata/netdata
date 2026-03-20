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

When you sign in using your email address without a password, Netdata Cloud sends a magic link to your email. This link allows you to authenticate securely without entering a password.

:::tip

Magic links expire after 15 minutes for security. If your link expires, you can request a new one from the sign-in page.

:::

Magic links can be used from multiple devices within the expiration window. For example, if you receive the link on your phone, you can open it on your desktop to complete authentication. Opening a link on one device does not invalidate it on another device until the 15-minute expiration period ends.

## Authorization

Once logged in, you can manage role-based access in your Space to give each team member the appropriate role. For more information, see [Role-Based Access model](/docs/netdata-cloud/authentication-and-authorization/role-based-access-model.md).
