# Top tab

The Top tab allows you to run [Netdata Functions](https://github.com/netdata/netdata/blob/master/docs/cloud/netdata-functions.md) on a node where a Netdata Agent is running. These routines are exposed by a given collector.
They can be used to retrieve additional information to help you troubleshoot or to trigger some action to happen on the node itself.

> **Tip**
>
> You can also execute a Function from the [Nodes tab](https://github.com/netdata/netdata/blob/master/docs/dashboard/nodes-tab.md), by pressing the `f(x)` button.

> **Note**
>
> If you get an error saying that your node can't execute Functions please check the [prerequisites](https://github.com/netdata/netdata/blob/master/docs/cloud/netdata-functions.md#prerequisites).

The main view of this tab provides you with (depending on the Function) two elements: a visualization on the top and a table on the bottom.

Visualizations vary depending on the Function and most allow for user customization.

On the top right-hand corner you can:

- Refresh the results (Given that the dashboard is on `Paused` mode)
- Set the update interval of the results.

## Functions bar

The bar on the right-hand side allows you to select which Function to run, on which node, and then depending on the Function, there might be more fine-grained filtering available.

For example the `Block-devices` Function allows you to filter per Device, Type, ID, Model and Serial number or the Block devices on your node.
