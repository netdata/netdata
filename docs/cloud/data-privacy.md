# Data privacy in Netdata Cloud

[Data privacy](https://netdata.cloud/privacy/) is very important to us. We firmly believe that your data belongs to
you. This is why **we don't store any metric data in Netdata Cloud**.

Your local installations of the Netdata Agent form the basis for the Netdata Cloud. All the data that you see in the web browser when using Netdata Cloud, is actually streamed directly from the Netdata Agent to the Netdata Cloud dashboard.
The data passes through our systems, but it isn't stored. You can learn more about [the Agent's security design](https://github.com/netdata/netdata/blob/master/docs/netdata-security.md) in the Agent documentation.

However, to be able to offer the stunning visualizations and advanced functionality of Netdata Cloud, it does store a limited number of _metadata_.

## Data Netdata Cloud stores and processes

Let's look at the metadata Netdata Cloud stores using the publicly available demo server `frankfurt.my-netdata.io`:

- The email address you used to sign up/or sign in
- For each node connected to your Spaces in Netdata Cloud:
  - Hostname (as it appears in Netdata Cloud)
  - Information shown in `/api/v1/info`. For example: [https://frankfurt.my-netdata.io/api/v1/info](https://frankfurt.my-netdata.io/api/v1/info).
  - Metric metadata information shown in `/api/v1/contexts`. For example: [https://frankfurt.my-netdata.io/api/v1/contexts](https://frankfurt.my-netdata.io/api/v1/contexts).
  - Alarm configurations shown in `/api/v1/alarms?all`. For example: [https://frankfurt.my-netdata.io/api/v1/alarms?all](https://frankfurt.my-netdata.io/api/v1/alarms?all).
  - Active alarms shown in `/api/v1/alarms`. For example: [https://frankfurt.my-netdata.io/api/v1/alarms](https://frankfurt.my-netdata.io/api/v1/alarms).

How we use them:

- The data is stored in our production database on AWS. Some of it is also used in Google BigQuery, our data lake, for analytics purposes. These analytics are crucial for our product development process.
- Email is used to identify users in regards to product use and to enrich our tools with product use, such as our CRM. 
- This data is only available to Netdata and never to a 3rd party.

## Delete all personal data

To remove all personal info we have about you (email and activities) you need to delete your cloud account by logging into https://app.netdata.cloud and accessing your profile, at the bottom left of your screen.
