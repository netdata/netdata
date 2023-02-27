# Agent Dashboards

Because Netdata is a health monitoring and _performance troubleshooting_ system,
we put a lot of emphasis on real-time, meaningful, and context-aware charts.

We bundle Netdata with a dashboard and hundreds of charts, designed by both our
team and the community, but you can also customize them yourself.

There are two primary ways to view Netdata's dashboards on the agent:

1.  The [local Agent dashboard](https://github.com/netdata/netdata/blob/master/web/gui/README.md) that comes pre-configured with every Netdata installation. You can
    see it at `http://NODE:19999`, replacing `NODE` with `localhost`, the hostname of your node, or its IP address. You
    can customize the contents and colors of the standard dashboard [using
    JavaScript](https://github.com/netdata/netdata/blob/master/web/gui/README.md#customizing-the-local-dashboard).

2.  The [`dashboard.js` JavaScript library](#dashboardjs), which helps you
   [customize the standard dashboards](https://github.com/netdata/netdata/blob/master/web/gui/README.md#customizing-the-local-dashboard)
   using JavaScript, or create entirely new [custom dashboards](https://github.com/netdata/netdata/blob/master/web/gui/custom/README.md) or
   [Atlassian Confluence dashboards](https://github.com/netdata/netdata/blob/master/web/gui/confluence/README.md).

You can also view all the data Netdata collects through the [REST API v1](https://github.com/netdata/netdata/blob/master/web/api/README.md#netdata-rest-api).

## dashboard.js

Netdata uses the `dashboards.js` file to define, configure, create, and update
all the charts and other visualizations that appear on any Netdata dashboard.
You need to put `dashboard.js` on any HTML page that's going to render Netdata
charts.

The [custom dashboards documentation](https://github.com/netdata/netdata/blob/master/web/gui/custom/README.md) contains examples of such
custom HTML pages.

### Generating dashboard.js

We build the `dashboards.js` file by concatenating all the source files located
in the `web/gui/src/dashboard.js/` directory. That's done using the provided
build script:

```sh
cd web/gui
make
```

If you make any changes to the `src` directory when developing Netdata, you
should regenerate the `dashboard.js` file before you commit to the Netdata
repository.
