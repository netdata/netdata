# Dashboards tab

With Netdata Cloud, you can build **custom dashboards** that target your infrastructure's unique needs. Put key metrics from any number of distributed systems in one place for a bird's eye view of your infrastructure.

Click on the **Dashboards** tab in any War Room to get started.

## Create your first dashboard

From the Dashboards tab, click on the **+** button.

In the modal, give your custom dashboard a name, and click **+ Add**.

- The **Add Chart** button on the top right of the interface adds your first chart card. From the dropdown, select either **All Nodes** or a specific node.  
  
  Next, select the context. You'll see a preview of the chart before you finish adding it. In this modal you can also [interact with the chart](https://github.com/netdata/netdata/blob/master/docs/dashboards-and-charts/netdata-charts.md), meaning you can configure all the aspects of the [NIDL framework](https://github.com/netdata/netdata/blob/master/docs/dashboards-and-charts/netdata-charts.md#nidl-framework) of the chart and more in detail, you can:
  - define which `group by` method to use
  - select the aggregation function over the data source
  - select nodes
  - select instances
  - select dimensions
  - select labels
  - select the aggregation function over time
  
  After you are done configuring the chart, you can also change the type of the chart from the right hand side of the [Title bar](https://github.com/netdata/netdata/blob/master/docs/dashboards-and-charts/netdata-charts.md#title-bar), and select which of the final dimensions you want to be visible and in what order, from the [Dimensions bar](https://github.com/netdata/netdata/blob/master/docs/dashboards-and-charts/netdata-charts.md#dimensions-bar).

- The **Add Text** button on the top right of the interface creates a new card with user-defined text, which you can use to describe or document a particular dashboard's meaning and purpose.

> ### Important
>
> Be sure to click the **Save** button any time you make changes to your dashboard.

## Using your dashboard

Dashboards are designed to be interactive and flexible so you can design them to your needs. They are made from any number of charts and cards, which can contain charts or text.

### Charts

The charts you add to any dashboard are [fully interactive](https://github.com/netdata/netdata/blob/master/docs/dashboards-and-charts/netdata-charts.md), just like any other Netdata chart. You can zoom in and out, highlight timeframes, and more.

Charts also synchronize as you interact with them, even across contexts _or_ nodes.

### Text cards

You can use text cards as notes to explain to other members of the [War Room](https://github.com/netdata/netdata/blob/master/docs/netdata-cloud/organize-your-infrastructure-invite-your-team.md#netdata-cloud-war-rooms) the purpose of the dashboard's arrangement.

By clicking the `T` icon on the text box, you can switch between font sizes.

### Move elements

To move a chart or a card, click and hold on **Drag & drop** at the top right of each element and drag it to a new location. A green placeholder indicates the
new location. Once you release your mouse, other elements re-sort to the grid system automatically.

### Resize elements

To resize any element on a dashboard, click on the bottom-right corner and drag it to its new size. Other elements re-sort to the grid system automatically.

### Go to chart

Quickly jump to the location of the chart in either the [Metrics tab](https://github.com/netdata/netdata/blob/master/docs/dashboards-and-charts/metrics-tab-and-single-node-tabs.md) or if the chart refers to a single node, its single node dashboard by clicking the 3-dot icon in the corner of any chart to open a menu. Hit the **Go to Chart** item.

You'll land directly on that chart of interest, but you can now scroll up and down to correlate your findings with other
charts. Of course, you can continue to zoom, highlight, and pan through time just as you're used to with Netdata Charts.

### Rename a chart

Using the 3-dot icon in the corner of any chart, you can rename it to better explain your use case or the visualization settings you've chosen for the chart.

### Remove an individual element

Click on the 3-dot icon in the corner of any card to open a menu. Click the **Remove** item to remove the card.

## Managing your dashboard

To see dashboards associated with the current War Room, click the **Dashboards** tab in any War Room. You can select dashboards and delete them using the 🗑️ icon.

### Update/save a dashboard

If you've made changes to a dashboard, such as adding or moving elements, the **Save** button is enabled. Click it to save your most recent changes.

Any other members of the War Room will be able to see these changes the next time they load this dashboard.

If multiple users attempt to make concurrent changes to the same dashboard, the second user who hits Save will be
prompted to either overwrite the dashboard or reload to see the most recent changes.

### Delete a dashboard

Delete any dashboard by navigating to it and clicking the **Delete** button. This will remove this entry from the
dropdown for every member of this War Room.

### Minimum browser viewport

Because of the visual complexity of individual charts, dashboards require a minimum browser viewport of 800px.

## What's next?

Once you've designed a dashboard or two, make sure to [invite your team](https://github.com/netdata/netdata/blob/master/docs/netdata-cloud/organize-your-infrastructure-invite-your-team.md#invite-your-team) if you haven't already. You can add these new users to the same War Room to let them see the same dashboards without any effort.
