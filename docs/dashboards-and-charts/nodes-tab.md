# Nodes tab

The nodes tab provides a summarized view of your [War Room](https://github.com/netdata/netdata/blob/master/docs/cloud/manage/organize-your-infrastrucutre-invite-your-team.md#netdata-cloud-war-rooms), allowing you to view quick information per node.

> **Tip**  
>
> Keep in mind that all configurations mentioned below are persistent and visible across all users.

## Center information view

The center information view consists of one row per node, and can be configured and filtered by the user.

### Filtering and adjusting the view

In the top right-hand corner, you can:

- Order the nodes per status or per alert status
- Select which charts you want to be displayed as quick reference points

### Node row

Each node row allows you to:

- View the node's status
- Go to a single node dashboard, by clicking the node name
- View information about the node, along with a button to display more in the right-hand sidebar
- View active alerts for the node
- View Machine Learning status
- View Functions capability status
- Add configuration (beta)
- [Add alert silencing rules](https://github.com/netdata/netdata/blob/master/docs/alerts-and-notifications/notifications/centralized/cloud/notifications/manage-alert-notification-silending-rules.md)
- View a set of key attributes collected on your node

## Right bar

The bar on the right-hand side provides additional information about the nodes in the War Room and allows you to filter what is displayed in the [center information view](#center-information-view).

### Node hierarchy

The first tab displays a hierarchy of the nodes displayed, making it easy to find a specific node by name. It follows the ordering that the user has selected.

### Filters sub-tab

The second tab allows you to filter which nodes are displayed, you can filter by:

- Host labels
- Node status
- Netdata version
- Individual nodes

### Alerts sub-tab

The third tab displays room alerts and allows you to see additional information about each alert.

### Info sub-tab

The last tab presents information about a node, by clicking the `i` icon from a node's row, right next to its name.
