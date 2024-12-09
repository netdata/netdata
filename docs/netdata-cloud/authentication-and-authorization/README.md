# Authentication & Authorization

This section contains documentation about Authentication and the Authorization flows that control the access and actions of their teammates in Netdata Cloud.

## Authentication

| method                        | description                                                                                                                                                                                                                                                 |
|:------------------------------|:------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| Email                         | To sign-in/sign-up using email, visit [Netdata Cloud](https://app.netdata.cloud/sign-in?cloudRoute=spaces?utm_source=docs&utm_content=sign_in_button_email_section), and follow the instructions                                                            |
| Google and GitHub OAuth       | To sign in/sign up using Google or GitHub OAuth, visit [Netdata Cloud](https://app.netdata.cloud/sign-in?cloudRoute=spaces?utm_source=docs&utm_content=sign_in_button_google_github_section), select the method you want to use and follow the instructions |
| Enterprise SSO Authentication | Netdata integrates with SSO tools, allowing you to control how your team connects and authenticates to Netdata Cloud, [read more](/docs/netdata-cloud/authentication-and-authorization/enterprise-sso-authentication.md).                                   |

> **Note**
>
> When you use Google/GitHub OAuth, your Netdata Cloud account is associated with the email address that it receives through OAuth.

## Authorization

Once logged in, you can manage role-based access in your space to give each team member the appropriate role. For more information, see [Role-Based Access model](/docs/netdata-cloud/authentication-and-authorization/role-based-access-model.md).
