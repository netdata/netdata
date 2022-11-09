<!--
title: "Nodes"
sidebar_label: "Nodes"
custom_edit_url: "https://github.com/netdata/netdata/blob/master/docs/tasks/setup/space-administration/nodes.md"
learn_status: "Published"
learn_topic_type: "Tasks"
sidebar_position: "10"
learn_rel_path: "Setup/Space administration"
learn_docs_purpose: "Instructions on how an admin can configure a space"
-->

As an admin you can configure a Space to your liking, through the Centralized Admin Interface accessible from the bottom
left cogwheel on the interface, with tooltip "Space Settings".

In this Task you will learn how to:

- [Add a description](#add-a-description)
- [Manage Permissions](#manage-permissions)
- [Create a new War Room](#create-a-new-war-room)
- [Delete a War Room](#delete-a-war-room)
- [Check the State of the Space's Nodes](#check-the-state-of-the-spaces-nodes)
- [Claim a node to the Space](#claim-a-node-to-the-space)
- [Remove a node from the Space](#remove-a-node-from-the-space)
- [Add a user to the Space](#add-a-user-to-the-space)
- [Remove a user from the Space](#remove-a-user-from-the-space)
- [Change a user's role within the Space](#change-a-users-role-within-the-space)
- [Enable/Disable Space-wide notifications](#enabledisable-space-wide-notifications)
- [Create/Delete Space-wide Bookmarks](#createdelete-space-wide-bookmarks)
- [Delete or Leave the Space](#delete-or-leave-the-space)

## Prerequisites

To perform administration on a node, you need the following:

- A Netdata Cloud account with at least one space where it has admin access
- (optional) A node claimed to that space

## Add a description

You can add a description of your Space's purpose in the **Info** tab.

## Manage permissions

Within the **Info** tab, you can limit the invitation of new members and of War Room creation to either
all the users or only the admin.

## Create a new War Room

To create a new war room:

1. Navigate to the **War Rooms** tab.
2. Click the green **+** icon **Create War Room**
3. Proceed into giving a name to the War Room
4. (Optional) Give a description to the War Room
5. Click **Add** to create the new War Room

## Delete a War Room

While in the **War Rooms** tab, you can see all the War Rooms of the Space you are in, and their number of nodes and
users. To delete a War Room:

1. Go to the **War Rooms* tab.
2. Click the **trashcan** icon
3. In the confirmation message click **Yes**
4. The War Room is successfully deleted from your Space.

## Check the state of the Space's nodes

From the **Nodes** tab, you have access to all the Nodes claimed on this Space, for each of them you can see:

- The Node's name
- The Node's version
- The node's status
- The connection to cloud

So, you can see if a node is outdated, offline, or needs further steps to connect to the cloud.

## Claim a node to the Space

From the **Nodes** tab, you can click the green **+** icon to begin the claiming process.  
For a detailed guide in claiming Agent nodes, refer to
our [Claim an Agent to the Cloud Task](https://github.com/netdata/netdata/blob/master/docs/tasks/setup/claim-existing-agent-to-cloud.md)
.

## Remove a node from the Space

While in the **Nodes** tab, you can remove any given offline node from a Space, by clicking the trash can icon in the
**Actions** column.

## Add a user to the Space

From the **Users** tab you can add more users to your Space.  
To do so:

1. Click the green **+** icon
2. Enter the email(s) of the user(s) you want to add (separated with a comma for multiple emails)
3. Select in which War Rooms the user will have access
4. Click **Send**

## Remove a user from the Space

You can remove a User by going to the **Users** tab by clicking the trash can icon in the **Actions** column within the row that contains
their name.

## Change a user's role within the Space

While in the **Users** tab, you can change a user's role by clicking the user icon in the **Actions** column and 
selecting the role for that user.

## Enable/Disable Space-wide notifications

From the **Notifications** tab, you can enable or disable Space-wide E-mail notifications by clicking the respective
toggle button.

## Create/Delete Space-wide Bookmarks

In the **Bookmarks** tab, you can create and delete Bookmarks that will appear in the left bar of the Space at any
given time.

## Delete or Leave the Space

You can delete or leave your space by clicking either **Leave Space** or **Delete
Space** in the bottom of the **Info** tab.

## Related topics

### Related Concepts

- [Spaces](https://github.com/netdata/learn/blob/master/docs/concepts/netdata-cloud/spaces.md)
