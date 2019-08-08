# The standard Netdata web dashboard

The standard web dashbboard is the heart of Netdata's performance troubleshooting toolkit. You've probably seen it before:

![A GIF of the standard Netdata web dashboard](https://user-images.githubusercontent.com/2662304/48307727-9175c800-e55b-11e8-92d8-a581d60a4889.gif)

By default, Netdata starts a web server for its dashboard at port `19999`. Open up your web browser of choice and navigate to `http://SERVER-IP:19999`, or `http://localhost:19999` on `localhost`.

Netdata uses an [internal, static-threaded web server](../server/) to host the HTML, CSS, and JavaScript files that make up the standard dashboard. You don't have to configure anything to access it, although you can adjust [a number of settings](../server/#other-netdataconf-web-section-options) in the `netdata.conf` file, or run Netdata behind an Nginx proxy, and so on.

## Navigating the standard dashboard

Beyond charts, the standard dashboard can be broken down into a few key areas: sections, menus/submenus, and the nodes menu.

> > > Annotated screenshot of the dashboard

### Sections

Netdata is broken up into multiple **sections**, such as **System Overview**, **CPU**, **Disk**, and more. Inside each section you'll find a number of charts, broken down into [contexts](../README.md#contexts) and [families](../README.md#families).

> > > screenshot of a section

All sections and their associated charts appear on a single "page," so all you need to do to view different sections is scroll up and down the page. But it's usually quicker to use the [menu](#menu).

### Menus

**Menus** appears on the right-hand side of the standard dashboard. Netdata generates a menu for each section, and menus link to the section they're associated with.

> > > screenshot of the menu

Most menu items will contain a number of **submenu** entries, which represent any [families](../README.md#families) from that section. Netdata automatically generates these submenu entries.

Here's a **Disks** menu with a number of submenu entries for each disk drive and partition Netdata recognizes.

> > > screenshot of some submenus

### Nodes menu

The nodes menu appears in the top-left corner of the standard dashboard and is labeled with the hostname of the system Netdata is monitoring.

Clicking on it will display a drop-down menu of any nodes you might have connected together via the [Netdata registry](../../registry/). By default, you'll find nothing under the **My nodes** heading, but you can try out any of the demo Netdata nodes to see how the nodes menu works.

> > > replace this image

![Screenshot of the default (empty) nodes menu](https://user-images.githubusercontent.com/1153921/62741606-5cb27680-b9f0-11e9-9f77-517f321b4dd5.png)

Once you add nodes via [Netdata Cloud](../../docs/netdata-cloud/) or a [private registry](../../registry/#run-your-own-registry), you will see them appear under the **My nodes** heading.

> > > screenshot of a filled my nodes list

## Customizing the standard dashboard

Netdata stores information about individual charts in the `dashboard_info.js` file. This file includes section and subsection headings, descriptions, colors, titles, tooltips, and other information that's rendered on the dashboard.

For example, here is how `dashboard_info.js` defines the **System Overview** section.

```
netdataDashboard.menu = {
    'system': {
        title: 'System Overview',
        icon: '<i class="fas fa-bookmark"></i>',
        info: 'Overview of the key system metrics.'
    },
```

If you want to customize this information, you should avoid editing `dashboard_info.js` directly. These changes are not persistent; Netdata will overwrite the file when it's updated. Instead, you should create a new file with your customizations.

We created an example file at [`dashboard_info_custom_example.js`](dashboard_info_custom_example.js). You can copy this to a new file with a name of your choice in the `web/` directory.

```
sudo cp dashboard_info_custom_example.js your_dashboard_info_file.js
```

> > > > FiGURE OUT WHY THIS IS BROKEN. TRY ON LINUX.

Finally, tell Netdata where you've put your customization file by replacing `your_dashboard_info_file.js` below.

```
[web]
   custom dashboard_info.js = your_dashboard_info_file.js
```

## Custom dashboards

For information on creating custom dashboards from scratch, see the [custom dashboards](custom/) or [Atlassian Confluence dashboards](confluence/) guides.

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fweb%2Fgui%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
