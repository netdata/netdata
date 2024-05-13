# Authentication & Authorization

This section contains documentation about how Netdata allows users to Authenticate with Netdata Cloud, as well as the Authorization flows that control the access and actions of their teammates in Netdata Cloud.

## Authentication

### Email

To sign in/sign up using email, visit [Netdata Cloud](https://app.netdata.cloud/sign-in?cloudRoute=spaces?utm_source=docs&utm_content=sign_in_button_email_section), enter your email address, and click the **Sign in by email** button.

Click the **Verify** button in the email you received to start using Netdata Cloud.

### Google and GitHub OAuth

When you use Google/GitHub OAuth, your Netdata Cloud account is associated with the email address that Netdata Cloud receives through OAuth.

To sign in/sign up using Google or GitHub OAuth, visit [Netdata Cloud](https://app.netdata.cloud/sign-in?cloudRoute=spaces?utm_source=docs&utm_content=sign_in_button_google_github_section) select the method you want to use. After the verification steps, you will be signed in to Netdata Cloud.

### Enterprise SSO Authentication

Netdata integrates with SSO tools, allowing you to control how your team connects and authenticates to Netdata Cloud.

For more information, see [Enterprise SSO Authentication](https://github.com/netdata/netdata/blob/master/docs/netdata-cloud/enterprise-sso-authentication.md).

## Authorization

Once logged in, you can manage role-based access in your space to give each team member the appropriate role. For more information, see [Role-Based Access model](https://github.com/netdata/netdata/blob/master/docs/cloud/manage/role-based-access.md).
