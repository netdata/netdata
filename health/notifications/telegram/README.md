# Telegram Agent alert notifications

Learn how to send notifications to Telegram using Netdata's Agent alert notification feature, which supports dozens of endpoints, user roles, and more.

> ### Note
>
> This file assumes you have read the [Introduction to Agent alert notifications](https://github.com/netdata/netdata/blob/master/health/notifications/README.md), detailing how the Netdata Agent's alert notification method works.

[Telegram](https://telegram.org/) is a messaging app with a focus on speed and security, it’s super-fast, simple and free. You can use Telegram on all your devices at the same time — your messages sync seamlessly across any number of your phones, tablets or computers.

Telegram messages look like this:

<img src="https://user-images.githubusercontent.com/1153921/66612223-f07dfb80-eb75-11e9-976f-5734ffd93ecd.png" width="50%"></img>

Netdata will send warning messages without vibration.

## Prerequisites

You will need:

- A bot token. To get one, contact the [@BotFather](https://t.me/BotFather) bot and send the command `/newbot` and follow the instructions.
  Start a conversation with your bot or invite it into a group where you want it to send messages.
- The chat ID for every chat you want to send messages to. Contact the [@myidbot](https://t.me/myidbot) bot and send the `/getid` command to get your personal chat ID or invite it into a group and use the `/getgroupid` command to get the group chat ID. Group IDs start with a hyphen, supergroup IDs start with `-100`.  

    Alternatively, you can get the chat ID directly from the bot API. Send *your* bot a command in the chat you want to use, then check `https://api.telegram.org/bot{YourBotToken}/getUpdates`, eg.  `https://api.telegram.org/bot111122223:7OpFlFFRzRBbrUUmIjj5HF9Ox2pYJZy5/getUpdates`
- terminal access to the Agent you wish to configure

## Configure Netdata to send alert notifications to Telegram

> ### Info
>
> This file mentions editing configuration files.  
>
> - To edit configuration files in a safe way, we provide the [`edit config` script](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md#use-edit-config-to-edit-configuration-files) located in your [Netdata config directory](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md#the-netdata-config-directory) (typically is `/etc/netdata`) that creates the proper file and opens it in an editor automatically.  
> Note that to run the script you need to be inside your Netdata config directory.
>
> It is recommended to use this way for configuring Netdata.

Edit `health_alarm_notify.conf`, changes to this file do not require restarting Netdata:

1. Set `SEND_TELEGRAM` to `YES`.
2. Set `TELEGRAM_BOT_TOKEN` to your bot token.
3. Set `DEFAULT_RECIPIENT_TELEGRAM` to the chat ID you want the alert notifications to be sent to.  
    You can define multiple chat IDs like this: `49999333322 -1009999222255`.  
    All roles will default to this variable if left unconfigured.

You can then have different chats per **role**, by editing `DEFAULT_RECIPIENT_TELEGRAM` with the chat ID you want, in the following entries at the bottom of the same file:

```conf
role_recipients_telegram[sysadmin]="49999333324"
role_recipients_telegram[domainadmin]="49999333389"
role_recipients_telegram[dba]="-1009999222255"
role_recipients_telegram[webmaster]="-1009999222255 49999333389"
role_recipients_telegram[proxyadmin]="49999333344"
role_recipients_telegram[sitemgr]="49999333876"
```

An example of a working configuration would be:

```conf
#------------------------------------------------------------------------------
# telegram (telegram.org) global notification options

SEND_TELEGRAM="YES"
TELEGRAM_BOT_TOKEN="111122223:7OpFlFFRzRBbrUUmIjj5HF9Ox2pYJZy5"
DEFAULT_RECIPIENT_TELEGRAM="-100233335555"
```

## Test the notification method

To test this alert notification method refer to the ["Testing Alert Notifications"](https://github.com/netdata/netdata/blob/master/health/notifications/README.md#testing-alert-notifications) section of the Agent alert notifications page.
