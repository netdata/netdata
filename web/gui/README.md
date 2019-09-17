# The standard web dashboard

The standard web dashboard is the heart of Netdata's performance troubleshooting
toolkit. You've probably seen it before:

![A GIF of the standard Netdata web
dashboard](https://user-images.githubusercontent.com/2662304/48307727-9175c800-e55b-11e8-92d8-a581d60a4889.gif)

Learn more about how dashboards work and how they're populated using the
`dashboards.js` file in our [web dashboards overview](../).

By default, Netdata starts a web server for its dashboard at port `19999`. Open
up your web browser of choice and navigate to `http://SERVER-IP:19999`, or
`http://localhost:19999` on `localhost`.

Netdata uses an [internal, static-threaded web server](../server/) to host the
HTML, CSS, and JavaScript files that make up the standard dashboard. You don't
have to configure anything to access it, although you can adjust [your
settings](../server/#other-netdataconf-web-section-options) in the
`netdata.conf` file, or run Netdata behind an Nginx proxy, and so on.

## Navigating the standard dashboard

Beyond charts, the standard dashboard can be broken down into three key areas:

1.  [**Sections**](#sections)
2.  [**Menus/submenus**](#menus)
3.  [**Nodes menu**](#nodes-menu)

![Annotated screenshot of the
dashboard](https://user-images.githubusercontent.com/1153921/62810777-ef681980-bab3-11e9-9310-ec7a0c9f4faa.png)

### Sections

Netdata is broken up into multiple **sections**, such as **System Overview**,
**CPU**, **Disk**, and more. Inside each section you'll find a number of charts,
broken down into [contexts](../#contexts) and
[families](../#families).

An example of the **Memory** section on a Linux desktop system.

![Screenshot of the Memory section of the Netdata
dashboard](https://user-images.githubusercontent.com/1153921/62811044-1410c100-bab5-11e9-91cc-414cf6c23985.png)

All sections and their associated charts appear on a single "page," so all you
need to do to view different sections is scroll up and down the page. But it's
usually quicker to use the [menus](#menus).

### Menus

**Menus** appears on the right-hand side of the standard dashboard. Netdata
generates a menu for each section, and menus link to the section they're
associated with.

![Screenshot of the
menu](https://user-images.githubusercontent.com/1153921/62811361-38b96880-bab6-11e9-8d41-4d9b29778e86.png)

Most menu items will contain several **submenu** entries, which represent any
[families](../#families) from that section. Netdata automatically
generates these submenu entries.

Here's a **Disks** menu with several submenu entries for each disk drive and
partition Netdata recognizes.

![Screenshot of some
submenus](https://user-images.githubusercontent.com/1153921/62811428-70281500-bab6-11e9-9a61-ecd0a7aba453.png)

### Nodes menu

The nodes menu appears in the top-left corner of the standard dashboard and is
labeled with the hostname of the system Netdata is monitoring.

Clicking on it will display a drop-down menu of any nodes you might have
connected via the [Netdata registry](../../registry/). By default, you'll find
nothing under the **My nodes** heading, but you can try out any of the demo
Netdata nodes to see how the nodes menu works.

![Screenshot of the default (empty) nodes
menu](https://user-images.githubusercontent.com/1153921/62795508-c3d13900-ba8b-11e9-98ed-f0be1b201340.png)

Once you add nodes via [Netdata Cloud](../../docs/netdata-cloud/) or a [private
registry](../../registry/#run-your-own-registry), you will see them appear under
the **My nodes** heading.

![Screenshot of a nodes menu populated by registry
agents](https://user-images.githubusercontent.com/1153921/62795509-c3d13900-ba8b-11e9-8459-e85bf71f7ceb.png)

The nodes menu will also show the master netdata node and all slave nodes
streaming to that master, if you have [configured streaming](../../streaming).

![Screenshot of a nodes menu populated by streaming
agents.](https://user-images.githubusercontent.com/1153921/62965774-90a7e600-bdba-11e9-8b80-495f35b29ecb.png)

## Customizing the standard dashboard

Netdata stores information about individual charts in the `dashboard_info.js`
file. This file includes section and subsection headings, descriptions, colors,
titles, tooltips, and other information for Netdata to render on the dashboard.

For example, here is how `dashboard_info.js` defines the **System Overview**
section.

```javascript
netdataDashboard.menu = {
  'system': {
    title: 'System Overview',
    icon: '<i class="fas fa-bookmark"></i>',
    info: 'Overview of the key system metrics.'
  },
```

If you want to customize this information, you should avoid editing
`dashboard_info.js` directly. These changes are not persistent; Netdata will
overwrite the file when it's updated. Instead, you should create a new file with
your customizations.

We created an example file at
[`dashboard_info_custom_example.js`](dashboard_info_custom_example.js). You can
copy this to a new file with a name of your choice in the `web/` directory. This
directory changes based on your operating system and installation method. If
you're on a Linux system, it should be at `/usr/share/netdata/web/`.

```shell
cd /usr/share/netdata/web/
sudo cp dashboard_info_custom_example.js your_dashboard_info_file.js
```

Edit the file with your customizations. For example:

```javascript
customDashboard.menu = {
  system: {
    title: "Testing, testing, 1 2 3",
    icon: '<i class="fas fa-thumbs-up"></i>',
    info: "This is overwritten info for the system overview section!"
  }
};
```

Finally, tell Netdata where you placed your customization file by replacing
`your_dashboard_info_file.js` below.

```conf
[web]
 custom dashboard_info.js = your_dashboard_info_file.js
```

Once you restart Netdata, refresh the dashboard to find your custom
configuration:

![Screenshot of overwritten text from dashboard_info.js
file](https://user-images.githubusercontent.com/1153921/62798924-570e6c80-ba94-11e9-9578-869753bec39c.png)

## Custom dashboards

For information on creating custom dashboards from scratch, see the [custom
dashboards](custom/) or [Atlassian Confluence dashboards](confluence/) guides.

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fweb%2Fgui%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)]()
