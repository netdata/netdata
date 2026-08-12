# Choose your Netdata UI theme

Netdata provides Dark and Light themes for the complete user interface. The theme changes the colors used by navigation,
dashboards, charts, tables, forms, and troubleshooting views; it does not change the metrics, queries, alerts, or other
monitoring data shown on those pages. Dark is the default theme.

## Change the theme

To change your theme across the Netdata UI, click on your profile picture, click on the **Settings**
tab, and then choose your preferred theme: **Light** or **Dark**.

The change applies across the Netdata UI, so you do not need to configure individual dashboards or charts. If you are
comparing screenshots or sharing a dashboard during an incident, remember that another user may have selected the other
theme. The underlying chart values and alert states are identical even when their colors differ.

## Choose a theme

Both themes expose the same controls and data. Choose the one that makes labels, chart lines, status indicators, and table
values easiest to read in your environment.

- **Dark** uses a dark background and is the default. It can reduce the brightness of a dashboard in a dim room or on a
  display that remains open for long periods.
- **Light** uses a light background. It can be easier to read in bright environments and can produce clearer screenshots
  for documents that use a white page background.

After switching, inspect a dashboard that contains several chart colors and alert states. Also check table text, tooltips,
and any custom dashboard content you rely on. Display calibration, browser contrast settings, operating-system color
filters, and room lighting can all affect readability, so there is no universally best choice.

## Dark theme

The Dark theme uses the following appearance:

![Dark theme](https://github.com/netdata/netdata/assets/70198089/81addd13-28a4-425f-ae39-0f9de5199496)

## Light theme

The Light theme uses the following appearance:

![Light theme](https://github.com/netdata/netdata/assets/70198089/eb0fb8c1-5695-450a-8ba8-a185874e8496)

## If the theme is difficult to read

First switch to the other built-in theme and reload the page. If the problem affects only one browser, compare the page in
another browser and check whether an extension, forced dark mode, high-contrast setting, or color filter is overriding the
site's styles. Browser zoom can help with text size, but it does not alter the theme's color palette.

When reporting a display problem, include the selected Netdata theme, browser and version, operating system, and a
screenshot of the affected component. State whether the problem is limited to one chart or appears throughout the UI. That
information separates a theme contrast problem from a chart-specific color or rendering problem.
