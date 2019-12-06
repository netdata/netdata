# Step 3. Monitor more than one system with Netdata

The Netdata agent is _distributed_ by design. That means each agent operates independently from any other, collecting
and creating charts only for the system you installed it on. We made this decision a long time ago to [improve security
and performance](step-01.md).

You might be thinking, "So, now I have to remember all these IP addresses, and type them into my browser
manually, to move from one system to another? Maybe I should just make a bunch of bookmarks. What's a few more tabs
on top of the hundred I have already? 🤬"

We get it. That's why we built [Netdata Cloud](../netdata-cloud/README.md), which connects many distributed agents
together for a seamless experience when monitoring multiple systems.

All without remembering IPs or making a bunch of bookmarks.

> If you're interested in streaming the metrics from one Netdata agent to another, that's unfortunately not part of this
> tutorial. You'll want to reference our [streaming documentation](../../streaming/README.md) when you're finished with
> these steps.

Even if you don't have multiple systems right now, keep reading. The instructions to follow will show you how to test
out these features with Netdata demo servers. That way, you'll be able to experience one of Netdata's defining features
right away.

## What you'll learn in this step

In this step of the Netdata guid, we'll talk about the following:

-   [Why you should use Netdata Cloud](#why-use-netdata-cloud)
-   [Add nodes to your Netdata Cloud account](#add-nodes-to-your-netdata-cloud-account)
-   [Navigate between your nodes via the **My nodes** menu](#navigate-between-your-nodes-via-the-my-nodes-menu)
-   [Try out the Nodes View](#try-out-the-nodes-view)

## Why use Netdata Cloud?

We built Netdata Cloud to give users a way to bridge the gap between many distributed agents running concurrently, all
without creating a centralized database for all your systems' metrics.

> Read more: [_Introducing Netdata Cloud: our vision for distributed health and performance 
> monitoring_](https://blog.netdata.cloud/posts/netdata-cloud-announcement/).

Netdata Cloud gives you a better way to observe and take action on slowdowns, anomalies, or outages in your systems and
applications. It connects all your Netdata agents through your _web browser_, allowing you to move between different
nodes quickly and use the Nodes View to see a handful or hundreds of Netdata-monitored nodes on a single screen.

If you're keeping tabs on multiple systems with Netdata, Netdata Cloud gives you all the benefits of a centralized
monitoring solution while distributing the workload to each agent.

That makes Netdata Cloud both comprehensive and lightweight. The best of both worlds!

And, better yet, Netdata Cloud doesn't store any of your system's metrics. It stores _metadata_ about the system's IP,
hostname, and a randomly-created GUID, and nothing else. Metrics are streamed from your systems directly to your _web
browser_.

Essentially, your web browser hosts a SaaS application with all of Netdata Cloud's features embedded right into the
dashboard itself.

## Add nodes to your Netdata Cloud account

The best way to add nodes to your Netdata Cloud account is to click on the **Sign in** button on the top-right corner of
your Netdata dashboard.

That button will open a new tab in for Netdata Cloud, and will prompt you to log-in using email or authentication via
Google or GitHub.

If you chose email, Netdata Cloud will send you a "magic link" via email. Once you click on the link, that node will be
connected to your Netdata Cloud account and you'll be redirected back to your dashboard. If you chose Google or GitHub,
you'll be redirected back to your dashboard as soon as authentication is finished.

Here's what authentication via Google looks like:

![Animated GIF of signing in to Netdata Cloud via
Google](https://user-images.githubusercontent.com/1153921/65063750-bb460b00-d933-11e9-934c-b17e2b18f37c.gif)

Depending on your authentication method, your email address or name will appear in the top right of your dashboard
instead of the **Sign in** button.

At this point, you've successfully added a single Netdata agent to your Netdata Cloud account. _What about the rest?_

Well, all you have to do is visit another node and repeat the sign-in process.

Let's use a demo system as an example.

Visit the [Netdata website](https://www.netdata.cloud/#live-demo) and click on any of the gauge charts displayed
underneath the **Live Demo** header.

Once the dashboard loads, repeat the Netdata Cloud sign-in process. The demo server is now associated with your Netdata
Cloud account, and will appear in your **My nodes** menu.

Here's how the process looks in action:

![output-Peek 2019-09-17 10-44
mp4](https://user-images.githubusercontent.com/1153921/65066115-9d2ed980-d938-11e9-83c5-8127886dbe11.gif)

## Navigate between your nodes via the My nodes menu

Once you have multiple nodes added to Netdata Cloud, they will populate your **My nodes** menu. You can use this menu to
navigate between your systems quickly.

![Animated GIF of the My Nodes menu in
action](https://user-images.githubusercontent.com/1153921/65066485-483f9300-d939-11e9-87d0-b4718cb8122a.gif)

Whenever you pan, zoom, highlight, select, or pause a chart, Netdata will synchronize those settings with any other
agent you visit via the My nodes menu. Even your scroll position is synchronized, so you'll see the same charts and
respective data for easy comparisons or root cause analysis.

You can now seamlessly track performance anomalies across your entire infrastructure!

## Try out the Nodes View

Next, let's try out the Nodes View.

Nodes View is a feature built in to Netdata Cloud that offers a different interface for viewing the health status of
multiple nodes.

> Learn more about all the features within Nodes View and what charts/metrics are represented there in our
> [documentation](../netdata-cloud/nodes-view.md).

You can visit Nodes View by navigating to `https://netdata.cloud/console` in your browser. Or, you can click on the
**Nodes View** button in any Netdata dashboard. If you're not logged in to Netdata Cloud yet, you'll be asked to log in
first.

![Animated GIF of loading the Nodes
View](https://user-images.githubusercontent.com/1153921/65066750-d7e54180-d939-11e9-9415-a8556ed99a02.gif)

The Nodes View shows an aggregated list of the nodes you connected to Netdata Cloud, and shows at-a-glance health status
for each.

Click on any of the boxes representing your nodes to see real-time, per-second charts of essential metrics in the **Node
overview** sidebar.

![output-Peek 2019-09-17 11-00
mp4](https://user-images.githubusercontent.com/1153921/65067327-192a2100-d93b-11e9-9824-80e142ac62c5.gif)

You can also view raised alarms and see real-time metrics from a [select number of
services/applications](../netdata-cloud/nodes-view.md#services-available-in-the-nodes-view) using the various tabs
available in the node overview sidebar.

If you add a large number of nodes to the Nodes View, you may want to look into the different view and sorting options.
You can choose between **full**, **compact**, and **detailed** view modes.

![Animated GIF of the various view
modes](https://user-images.githubusercontent.com/1153921/65068318-4bd51900-d93d-11e9-8720-b3bd76809d16.gif)

You can also sort between grouping nodes by hostname, recently viewed, or most frequestly visited. Or, group them by
alarm status, their services, or whether they're online or unreachable.

![Animated GIF of the sorting and grouping options in Nodes
View](https://user-images.githubusercontent.com/1153921/65068421-7b842100-d93d-11e9-8a9a-e2afb06a99f6.gif)

Play around until you find the right settings for you and your infrastructure.

### Remove a node from Nodes View

If you want to clean up your Nodes View a bit, you can remove them from your Netdata Cloud account.

Click on the node in question, and then scroll to the bottom of the Node overiew sidebar. You'll see a URL under the
**Node URLs** heading. Hover over the URL and click on the garbage bin icon. Click **Confirm** on the modal window.
Then, click the **Forget** button that appears in the sidebar, and hit **Confirm** once again.

![Removing a node from Nodes View](https://user-images.githubusercontent.com/1153921/68406518-357a5b00-013f-11ea-85b0-3dc797eb9ff8.gif)

## What's next?

Now that you know how to add multiple nodes to your Netdata Cloud agent and navigate between them, it's time to learn
more about how you can configure Netdata to your liking. From there, you'll be able to customize your Netdata experience
to your exact infrastructure and the information you need.

[Next: The basics of configuring Netdata &rarr;](step-04.md)
