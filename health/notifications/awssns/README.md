<!--
title: "Amazon SNS"
description: "hello"
custom_edit_url: https://github.com/netdata/netdata/edit/master/health/notifications/awssns/README.md
-->

# Amazon SNS

As part of it's AWS suite, Amazon provides a notification broker service called 'Simple Notification Service' or SNS.  Amazon SNS works kind of similarly to Netdata's own notification system, allowing dispatch of a single notification to multiple subscribers of different types.  Among other things, SNS supports sending notifications to:

-   Email addresses.
-   Mobile Phones via SMS.
-   HTTP or HTTPS web hooks.
-   AWS Lambda functions.
-   AWS SQS queues.
-   Mobile applications via push notifications.

To get this working, you will need:

-   The Amazon Web Services CLI tools.  Most distributions provide these with the package name `awscli`.
-   An actual home directory for the user you run Netdata as, instead of just using `/` as a home directory.  Setup of this is distribution specific.  `/var/lib/netdata` is the recommended directory (because the permissions will already be correct) if you are using a dedicated user (which is how most distributions work).
-   An Amazon SNS topic to send notifications to with one or more subscribers.  The [Getting
    Started](https://docs.aws.amazon.com/sns/latest/dg/sns-getting-started.html) section of the Amazon SNS documentation
    covers the basics of how to set this up.  Make note of the Topic ARN when you create the topic.
-   While not mandatory, it is highly recommended to create a dedicated IAM user on your account for Netdata to send notifications.  This user needs to have programmatic access, and should only allow access to SNS.  If you're really paranoid, you can create one for each system or group of systems.

Once you have all the above, run the following command as the user Netdata runs under:

```
aws configure
```

THis will prompt you for the access key and secret key for accessing Amazon SNS (as well as the default region and output format, but you can leave those blank because we don't use them).

Once that's done, you're ready to go and can specify the desired topic ARN as a recipient.

Notes:

-   Netdata's native email notification support is far better in almost all respects than it's support through Amazon SNS.  If you want email notifications, use the native support, not SNS.
    -   If you need to change the notification format for SNS notifications, you can do so by specifying the format in `AWSSNS_MESSAGE_FORMAT` in the configuration.  This variable supports all the same variables you can use in custom notifications.
    -   While Amazon SNS supports sending differently formatted messages for different delivery methods, Netdata does not currently support this functionality.


