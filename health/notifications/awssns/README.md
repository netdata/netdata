# Amazon SNS Agent alert notifications

Learn how to send notifications through Amazon SNS using Netdata's Agent alert notification feature, which supports dozens of endpoints, user roles, and more.

> ### Note
>
> This file assumes you have read the [Introduction to Agent alert notifications](https://github.com/netdata/netdata/blob/master/health/notifications/README.md), detailing how the Netdata Agent's alert notification method works.

As part of its AWS suite, Amazon provides a notification broker service called 'Simple Notification Service' (SNS). Amazon SNS works similarly to Netdata's own notification system, allowing to dispatch a single notification to multiple subscribers of different types. Among other things, SNS supports sending notifications to:

- email addresses
- mobile Phones via SMS
- HTTP or HTTPS web hooks
- AWS Lambda functions
- AWS SQS queues
- mobile applications via push notifications

> ### Note
>
> While Amazon SNS supports sending differently formatted messages for different delivery methods, Netdata does not currently support this functionality.

For email notification support, we recommend using Netdata's [email notifications](https://github.com/netdata/netdata/blob/master/health/notifications/email/README.md), as it is has the following benefits:

- In most cases, it requires less configuration.
- Netdata's emails are nicely pre-formatted and support features like threading, which requires a lot of manual effort in SNS.
- It is less resource intensive and more cost-efficient than SNS.

Read on to learn how to set up Amazon SNS in Netdata.

## Prerequisites

Before you can enable SNS, you need:

- The [Amazon Web Services CLI tools](https://docs.aws.amazon.com/cli/latest/userguide/getting-started-install.html) (`awscli`).
- An actual home directory for the user you run Netdata as, instead of just using `/` as a home directory.  
  The setup depends on the distribution, but `/var/lib/netdata` is the recommended directory. If you are using Netdata as a dedicated user, the permissions will already be correct.
- An Amazon SNS topic to send notifications to with one or more subscribers.  
  The [Getting Started](https://docs.aws.amazon.com/sns/latest/dg/sns-getting-started.html) section of the Amazon SNS documentation covers the basics of how to set this up. Make note of the **Topic ARN** when you create the topic.
- While not mandatory, it is highly recommended to create a dedicated IAM user on your account for Netdata to send notifications.  
  This user needs to have programmatic access, and should only allow access to SNS. For an additional layer of security, you can create one for each system or group of systems.
- Terminal access to the Agent you wish to configure.

## Configure Netdata to send alert notifications to Amazon SNS

> ### Info
>
> This file mentions editing configuration files.  
>
> - To edit configuration files in a safe way, we provide the [`edit config` script](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md#use-edit-config-to-edit-configuration-files) located in your [Netdata config directory](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md#the-netdata-config-directory) (typically is `/etc/netdata`) that creates the proper file and opens it in an editor automatically.  
> Note that to run the script you need to be inside your Netdata config directory.
>
> It is recommended to use this way for configuring Netdata.

Edit `health_alarm_notify.conf`, changes to this file do not require restarting Netdata:

1. Set `SEND_AWSNS` to `YES`.
2. Set `AWSSNS_MESSAGE_FORMAT` to the string that you want the alert to be sent into.

   The supported variables are:

   | Variable name               | Description                                                                      |
   |:---------------------------:|:---------------------------------------------------------------------------------|
   | `${alarm}`                  | Like "name = value units"                                                        |
   | `${status_message}`         | Like "needs attention", "recovered", "is critical"                               |
   | `${severity}`               | Like "Escalated to CRITICAL", "Recovered from WARNING"                           |
   | `${raised_for}`             | Like "(alarm was raised for 10 minutes)"                                         |
   | `${host}`                   | The host generated this event                                                    |
   | `${url_host}`               | Same as ${host} but URL encoded                                                  |
   | `${unique_id}`              | The unique id of this event                                                      |
   | `${alarm_id}`               | The unique id of the alarm that generated this event                             |
   | `${event_id}`               | The incremental id of the event, for this alarm id                               |
   | `${when}`                   | The timestamp this event occurred                                                |
   | `${name}`                   | The name of the alarm, as given in netdata health.d entries                      |
   | `${url_name}`               | Same as ${name} but URL encoded                                                  |
   | `${chart}`                  | The name of the chart (type.id)                                                  |
   | `${url_chart}`              | Same as ${chart} but URL encoded                                                 |
   | `${family}`                 | The family of the chart                                                          |
   | `${url_family}`             | Same as ${family} but URL encoded                                                |
   | `${status}`                 | The current status : REMOVED, UNINITIALIZED, UNDEFINED, CLEAR, WARNING, CRITICAL |
   | `${old_status}`             | The previous status: REMOVED, UNINITIALIZED, UNDEFINED, CLEAR, WARNING, CRITICAL |
   | `${value}`                  | The current value of the alarm                                                   |
   | `${old_value}`              | The previous value of the alarm                                                  |
   | `${src}`                    | The line number and file the alarm has been configured                           |
   | `${duration}`               | The duration in seconds of the previous alarm state                              |
   | `${duration_txt}`           | Same as ${duration} for humans                                                   |
   | `${non_clear_duration}`     | The total duration in seconds this is/was non-clear                              |
   | `${non_clear_duration_txt}` | Same as ${non_clear_duration} for humans                                         |
   | `${units}`                  | The units of the value                                                           |
   | `${info}`                   | A short description of the alarm                                                 |
   | `${value_string}`           | Friendly value (with units)                                                      |
   | `${old_value_string}`       | Friendly old value (with units)                                                  |
   | `${image}`                  | The URL of an image to represent the status of the alarm                         |
   | `${color}`                  | A color in  AABBCC format for the alarm                                          |
   | `${goto_url}`               | The URL the user can click to see the netdata dashboard                          |
   | `${calc_expression}`        | The expression evaluated to provide the value for the alarm                      |
   | `${calc_param_values}`      | The value of the variables in the evaluated expression                           |
   | `${total_warnings}`         | The total number of alarms in WARNING state on the host                          |
   | `${total_critical}`         | The total number of alarms in CRITICAL state on the host                         |

3. Set `DEFAULT_RECIPIENT_AWSSNS` to the Topic ARN you noted down upon creating the Topic.  
   All roles will default to this variable if left unconfigured.

You can then have different recipient Topics per **role**, by editing `DEFAULT_RECIPIENT_AWSSNS` with the Topic ARN you want, in the following entries at the bottom of the same file:

```conf
role_recipients_awssns[sysadmin]="arn:aws:sns:us-east-2:123456789012:Systems"
role_recipients_awssns[domainadmin]="arn:aws:sns:us-east-2:123456789012:Domains"
role_recipients_awssns[dba]="arn:aws:sns:us-east-2:123456789012:Databases"
role_recipients_awssns[webmaster]="arn:aws:sns:us-east-2:123456789012:Development"
role_recipients_awssns[proxyadmin]="arn:aws:sns:us-east-2:123456789012:Proxy"
role_recipients_awssns[sitemgr]="arn:aws:sns:us-east-2:123456789012:Sites"
```

An example working configuration would be:

```conf
#------------------------------------------------------------------------------
# Amazon SNS notifications

SEND_AWSSNS="YES"
AWSSNS_MESSAGE_FORMAT="${status} on ${host} at ${date}: ${chart} ${value_string}"
DEFAULT_RECIPIENT_AWSSNS="arn:aws:sns:us-east-2:123456789012:MyTopic"
```

## Test the notification method

To test this alert notification method refer to the ["Testing Alert Notifications"](https://github.com/netdata/netdata/blob/master/health/notifications/README.md#testing-alert-notifications) section of the Agent alert notifications page.
