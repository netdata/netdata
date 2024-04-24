# Enterprise SSO Authentication

Netdata provides you with means to streamline and control how your team connects and authenticates to Netdata Cloud. We provide
 diferent Single Sign-On (SSO) integrations that allow you to connect with the tool that your organization is using to manage your 
 user accounts.

 > ❗ This feature focus is on the Authentication flow, it doesn't support the Authorization with managing Users and Roles.


## How to set it up?

If you want to setup your Netdata Space to allow user Authentication through an Enterprise SSO tool you need to:
* Confirm the integration to the tool you want is available ([Authentication integations](https://learn.netdata.cloud/docs/netdata-cloud/authentication-&-authorization/cloud-authentication-&-authorization-integrations))
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

If you're starting your flow from Netdata sign-in page you need to:
1. Click on the link `Sign-in with an Enterprise Signle Sign-On (SSO)`
2. Enter your email address 
3. Go to your mailbox and check the `Sign In to Nedata` email that you have received
4. Click on the **Sign In** button

Note: If you're not authenticated on the Enterprise SSO tool you'll be prompted to authenticate there
first before being allowed to proceed to Netdata Cloud.
