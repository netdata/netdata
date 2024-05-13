# Authentication & Authorization

This section contains documentation about the way Netdata allows users to Authenticate with Netdata Cloud and the Authorization flows controlling what their teammates can access and do on Netdata Cloud.

## Authentication

### Email

To sign in/sign up with email, visit [Netdata Cloud](https://app.netdata.cloud/sign-in?cloudRoute=spaces?utm_source=docs&utm_content=sign_in_button_email_section), enter your email address, and click the **Sign in by email** button.

Click the **Verify** button in the email you received to start using Netdata Cloud.

### Google and GitHub OAuth

When you use Google/GitHub OAuth, your Netdata Cloud account is associated with the email address that Netdata Cloud
receives via OAuth.

To sign in/sign up with Google or GitHub OAuth, visit [Netdata Cloud](https://app.netdata.cloud/sign-in?cloudRoute=spaces?utm_source=docs&utm_content=sign_in_button_google_github_section) and select which method you want to use. After the verification steps, you will be signed in to Netdata Cloud.

### Enterprise SSO Authentication

Netdata integrates with SSO tools to allow you to control the way that your team can connect and authenticate with Netdata Cloud.

Check the section regarding [Enterprise SSO Authentication](https://github.com/netdata/netdata/blob/master/docs/netdata-cloud/enterprise-sso-authentication.md) for more details.

## Authorization

After you are logged in, you can manage role-based access in your Space to provide each of your team members with the appropriate role. Read more about our RBAC model in the [corresponding section of our docs](https://github.com/netdata/netdata/blob/master/docs/cloud/manage/role-based-access.md).
