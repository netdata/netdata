<!--
title: "Telegram agent alert notifications"
sidebar_label: "Telegram"
custom_edit_url: "https://github.com/netdata/netdata/edit/master/health/notifications/telegram/README.md"
learn_status: "Published"
learn_topic_type: "Tasks"
learn_rel_path: "Integrations/Notify/Agent alert notifications"
learn_autogeneration_metadata: "{'part_of_cloud': False, 'part_of_agent': True}"
-->

# Telegram agent alert notifications

[Telegram](https://telegram.org/) is a messaging app with a focus on speed and security, it’s super-fast, simple and free. You can use Telegram on all your devices at the same time — your messages sync seamlessly across any number of your phones, tablets or computers.

With Telegram, you can send messages, photos, videos and files of any type (doc, zip, mp3, etc), as well as create groups for up to 100,000 people or channels for broadcasting to unlimited audiences. You can write to your phone contacts and find people by their usernames. As a result, Telegram is like SMS and email combined — and can take care of all your personal or business messaging needs.

Netdata will send warning messages without vibration.

You need to:

1.  Get a bot token. To get one, contact the [@BotFather](https://t.me/BotFather) bot and send the command `/newbot`. Follow the instructions.
2.  Start a conversation with your bot or invite it into a group where you want it to send messages.
3.  Find the chat ID for every chat you want to send messages to. Contact the [@myidbot](https://t.me/myidbot) bot and send the `/getid` command to get your personal chat ID or invite it into a group and use the `/getgroupid` command to get the group chat ID. Group IDs start with a hyphen, supergroup IDs start with `-100`.  
    Alternatively, you can get the chat ID directly from the bot API. Send *your* bot a command in the chat you want to use, then check `https://api.telegram.org/bot{YourBotToken}/getUpdates`, eg.  `https://api.telegram.org/bot111122223:7OpFlFFRzRBbrUUmIjj5HF9Ox2pYJZy5/getUpdates`
4.  Set the bot token and the chat ID of the recipient in `/etc/netdata/health_alarm_notify.conf` (to edit it on your system run `/etc/netdata/edit-config health_alarm_notify.conf`), like this:
```
SEND_TELEGRAM="YES"
TELEGRAM_BOT_TOKEN="111122223:7OpFlFFRzRBbrUUmIjj5HF9Ox2pYJZy5"
DEFAULT_RECIPIENT_TELEGRAM="-100233335555"
```

You can define multiple recipients like this: `"-100311112222 212341234|critical"`.
This example will send:

-   All alerts to the group with ID -100311112222
-   Critical alerts to the user with ID 212341234

You can give different recipients per **role** using these (in the same file):

```
role_recipients_telegram[sysadmin]="212341234"
role_recipients_telegram[dba]="-1004444333321"
role_recipients_telegram[webmaster]="49999333322 -1009999222255"
```

Telegram messages look like this:

![Netdata notifications via Telegram](https://user-images.githubusercontent.com/1153921/66612223-f07dfb80-eb75-11e9-976f-5734ffd93ecd.png)


