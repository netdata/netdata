# Enterprise SSO Authentication

Netdata provides you with means to streamline and control how your team connects and authenticates to Netdata Cloud. We provide
 different Single Sign-On (SSO) integrations that allow you to connect with the tool that your organization is using to manage your
 user accounts.

 > **Note** This feature focus is on the Authentication flow, it doesn't support the Authorization with managing Users and Roles.

## How to set it up?

If you want to setup your Netdata Space to allow user Authentication through an Enterprise SSO tool you need to:

* Confirm the integration to the tool you want is available ([Authentication integrations](https://learn.netdata.cloud/docs/netdata-cloud/authentication-&-authorization/cloud-authentication-&-authorization-integrations))
* Have a Netdata Cloud account
* Have Access to the Space as an administrator
* Your Space needs to be on the Business plan or higher

Once you ensure the above prerequisites you need to:

1. Click on the Space settings cog (located above your profile icon)
2. Click on the Authentication tab
3. Select the card for the integration you are looking for, click on Configure
4. Fill the required attributes need to establish the integration with the tool

## How to authenticate to Netdata?

### From Netdata Sign-up page

#### Requirements

You have to update your DNS settings by adding a TXT record with the Netdata verification code as its **Value**.
The **Value** can be found by clicking on **DNS txt record** button, at your space settings on the **User Management** section, under **Authentication & Authorization** tab.

Log into your domain provider’s website, and navigate to the DNS records section.
Create a new TXT-record with the following specifications:
- Value/Answer/Description: `"netdata-verification=[VERIFICATION CODE]"`
- Name/Host/Alias: Leave this blank or type @ to include a subdomain.
- Time to live (TTL): "86400" (this can also be inherited from the default configuration).

#### Starting the flow from Netdata sign-in page

1. Click on the link `Sign-in with an Enterprise Single Sign-On (SSO)`
2. Enter your email address
3. Complete the SSO flow

Note: If you're not authenticated on the Enterprise SSO tool you'll be prompted to authenticate there
first before being allowed to proceed to Netdata Cloud.
