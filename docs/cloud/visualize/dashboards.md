# Build new dashboards

With Netdata Cloud, you can build new dashboards that target your infrastructure's unique needs. Put key metrics from
any number of distributed systems in one place for a bird's eye view of your infrastructure.

Click on the **Dashboards** tab in any War Room to get started.

## Create your first dashboard

From the Dashboards tab, click on the **+** button.

<img width="98" alt=" Green plus button " src="https://github.com/netdata/netdata/assets/73346910/511e2b38-e751-4a88-bc7d-bcd49764b7f6"/>


In the modal, give your new dashboard a name, and click **+ Add**.

- The **Add Chart** button on the top right of the interface adds your first chart card. From the dropdown, select either **All Nodes** or a specific
node. Next, select the context. You'll see a preview of the chart before you finish adding it. In this modal you can also [interact with the chart](https://github.com/netdata/netdata/blob/master/docs/cloud/visualize/interact-new-charts.md), meaning you can configure all the aspects of the [NIDL framework](https://github.com/netdata/netdata/blob/master/docs/cloud/visualize/interact-new-charts.md#nidl-framework) of the chart and more in detail, you can:
  - define which `group by` method to use
  - select the aggregation function over the data source
  - select nodes
  - select instances
  - select dimensions
  - select labels
  - select the aggregation function over time
  
  After you are done configuring the chart, you can also change the type of the chart from the right hand side of the [Title bar](https://github.com/netdata/netdata/blob/master/docs/cloud/visualize/interact-new-charts.md#title-bar), and select which of the final dimensions you want to be visible and in what order, from the [Dimensions bar](https://github.com/netdata/netdata/blob/master/docs/cloud/visualize/interact-new-charts.md#dimensions-bar).

- The **Add Text** button on the top right of the interface creates a new card with user-defined text, which you can use to describe or document a
particular dashboard's meaning and purpose.

> ### Important
>
> Be sure to click the **Save** button any time you make changes to your dashboard.


## Using your dashboard

Dashboards are designed to be interactive and flexible so you can design them to your exact needs. Dashboards are made
of any number of **cards**, which can contain charts or text.

### Chart cards

The charts you add to any dashboard are [fully interactive](https://github.com/netdata/netdata/blob/master/docs/cloud/visualize/interact-new-charts.md), just like any other Netdata chart. You can zoom in and out, highlight timeframes, and more.

Charts also synchronize as you interact with them, even across contexts _or_ nodes.

### Text cards

You can use text cards as notes to explain to other members of the [War Room](https://github.com/netdata/netdata/blob/master/docs/cloud/manage/organize-your-infrastrucutre-invite-your-team.md#netdata-cloud-war-rooms) the purpose of the dashboard's arrangement. 

### Move cards

To move any card, click and hold on **Drag & rearrange** at the top right of the card and drag it to a new location. A red placeholder indicates the
new location. Once you release your mouse, other charts re-sort to the grid system automatically.

### Resize cards

To resize any card on a dashboard, click on the bottom-right corner and drag to the card's new size. Other cards re-sort
to the grid system automatically.

## Go to chart

Quickly jump to the location of the chart in either the Overview tab or if the card refers to a single node, its single node dashboard by clicking the 3-dot icon in the corner of any card to open a menu. Hit the **Go to Chart** item.

You'll land directly on that chart of interest, but you can now scroll up and down to correlate your findings with other
charts. Of course, you can continue to zoom, highlight, and pan through time just as you're used to with Netdata Charts.

## Managing your dashboard

To see dashboards associated with the current War Room, click the **Dashboards** tab in any War Room. You can select
dashboards and delete them using the 🗑️ icon.

### Update/save a dashboard

If you've made changes to a dashboard, such as adding or moving cards, the **Save** button is enabled. Click it to save
your most recent changes. Any other members of the War Room will be able to see these changes the next time they load
this dashboard.

If multiple users attempt to make concurrent changes to the same dashboard, the second user who hits Save will be
prompted to either overwrite the dashboard or reload to see the most recent changes.

### Remove an individual card

Click on the 3-dot icon in the corner of any card to open a menu. Click the **Remove** item to remove the card.

### Delete a dashboard

Delete any dashboard by navigating to it and clicking the **Delete** button. This will remove this entry from the
dropdown for every member of this War Room.

### Minimum browser viewport

Because of the visual complexity of individual charts, dashboards require a minimum browser viewport of 800px.

## What's next?

Once you've designed a dashboard or two, make sure
to [invite your team](https://github.com/netdata/netdata/blob/master/docs/cloud/manage/organize-your-infrastrucutre-invite-your-team.md#invite-your-team) if
you haven't already. You can add these new users to the same War Room to let them see the same dashboards without any
effort.
