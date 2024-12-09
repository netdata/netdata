# Enterprise SSO Authentication

Netdata supports different Single Sign-On (SSO) integrations that allow you to connect with the tool your organization uses to manage user accounts.

> **Note**
>
> This feature's focus is on the Authentication flow, it doesn't support managing Users and Roles.

## Setup

To set up your Space to allow user Authentication through an Enterprise SSO tool, you need to:

* Check if an integration with Netdata exists for your integration tool (see [Authentication integrations](https://learn.netdata.cloud/docs/netdata-cloud/authentication-&-authorization/cloud-authentication-&-authorization-integrations))
* Have a Netdata Cloud account
* Have access to the Space as an Administrator
* Your Space needs to be on the Business plan or higher

Once you ensure the above prerequisites, you need to:

1. Click on the Space settings cog (located above your profile icon)
2. Click on User Management -> Authentication & Authorization
3. Select the card for the integration you’re looking for, click on Configure
4. Provide the required attributes needed to configure the integration with the tool

## Configuration on the domain provider's side

You have to update your DNS settings by adding a TXT record with the Netdata verification code as its **Value**.

The **Value** can be found by clicking the **DNS TXT record** button under your Space settings under **User Management**, in the**Authentication & Authorization** tab.

Log into your domain provider’s website, and navigate to the DNS records section. Create a new TXT record with the following specifications:

* Value/Answer/Description: `"netdata-verification=[VERIFICATION CODE]"`
* Name/Host/Alias: Leave this blank or type @ to include a subdomain.
* Time to live (TTL): "86400" (this can also be inherited from the default configuration).

## How to Authenticate

Click on the link `Sign-in with an Enterprise Single Sign-On (SSO)` and follow the instructions. If you're not authenticated on the Enterprise SSO tool, you'll be prompted to authenticate there first before being allowed to proceed to Netdata Cloud.
