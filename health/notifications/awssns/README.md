<!--
title: "Amazon SNS"
description: "hello"
custom_edit_url: https://github.com/netdata/netdata/edit/master/health/notifications/awssns/README.md
-->

# Amazon SNS

As part of its AWS suite, Amazon provides a notification broker service called 'Simple Notification Service' (SNS). Amazon SNS works similarly to Netdata's own notification system, allowing to dispatch a single notification to multiple subscribers of different types. While Amazon SNS supports sending differently formatted messages for different delivery methods, Netdata does not currently support this functionality.
Among other things, SNS supports sending notifications to:

-   Email addresses.
-   Mobile Phones via SMS.
-   HTTP or HTTPS web hooks.
-   AWS Lambda functions.
-   AWS SQS queues.
-   Mobile applications via push notifications.

For email notification support, we recommend using Netdata's email notifications, as it is has the following benefits:

- In most cases, it requires less configuration.
- Netdata's emails are nicely pre-formatted and support features like threading, which requires a lot of manual effort in SNS.
- It is less resource intensive and more cost-efficient than SNS. 

Read on to learn how to set up Amazon SNS in Netdata.

## Prerequisites

Before you can enable SNS, you need:

-   The [Amazon Web Services CLI tools](https://docs.aws.amazon.com/cli/latest/userguide/getting-started-install.html) (`awscli`).
-   An actual home directory for the user you run Netdata as, instead of just using `/` as a home directory. The setup depends on the distribution, but `/var/lib/netdata` is the recommended directory. If you are using Netdata as a dedicated user, the permissions will already be correct.
-   An Amazon SNS topic to send notifications to with one or more subscribers. The [Getting Started](https://docs.aws.amazon.com/sns/latest/dg/sns-getting-started.html) section of the Amazon SNS documentation covers the basics of how to set this up. Make note of the **Topic ARN** when you create the topic.
-   While not mandatory, it is highly recommended to create a dedicated IAM user on your account for Netdata to send notifications. This user needs to have programmatic access, and should only allow access to SNS. For an additional layer of security, you can create one for each system or group of systems.

## Enabling Amazon SNS

To enable SNS:
1. Run the following command as the user Netdata runs under:
   ```
   aws configure
   ```
2. Enter the access key and secret key for accessing Amazon SNS. The system also prompts you to enter the default region and output format, but you can leave those blank because Netdata doesn't use them.

3. Specify the desired topic ARN as a recipient, see [SNS documentation](https://docs.aws.amazon.com/AmazonCloudWatch/latest/monitoring/US_SetupSNS.html#set-up-sns-topic-cli).
4. Optional: To change the notification format for SNS notifications, change the `AWSSNS_MESSAGE_FORMAT` variable in `health_alarm_notify.conf`. 
This variable supports all the same variables you can use in custom notifications.

   The default format looks like this: 
   ```bash
   AWSSNS_MESSAGE_FORMAT="${status} on ${host} at ${date}: ${chart} ${value_string}"
   ```
   
