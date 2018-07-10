// SPDX-License-Identifier: GPL-3.0+
/*
 * Custom netdata information file
 * -------------------------------
 *
 * Use this file to add custom information on netdata dashboards:
 *
 * 1. Copy it to a new filename (so that it will not be overwritten with netdata updates)
 * 2. Edit it to fit your needs
 * 3. Set the following option to /etc/netdata/netdata.conf :
 *
 *    [web]
 *      custom dashboard_info.js = your_filename.js
 *
 * Using this file you can:
 *
 * 1. Overwrite or add messages to menus, submenus and charts.
 *    Use dashboard_info.js to find out what you can define.
 *
 * 2. Inject javascript code into the default netdata dashboard.
 *
 */

// ----------------------------------------------------------------------------
// MENU
//
// - title      the menu title as to be rendered at the charts menu
// - icon       html fragment of the icon to display
// - info       html fragment for the description above all the menu charts

customDashboard.menu = {

};


// ----------------------------------------------------------------------------
// SUBMENU
//
// - title       the submenu title as to be rendered at the charts menu
// - info        html fragment for the description above all the submenu charts

customDashboard.submenu = {

};


// ----------------------------------------------------------------------------
// CONTEXT (the template each chart is based on)
//
// - info        html fragment for the description above the chart
// - height      a ratio to the default as a decimal number: 1.0 = 100%
// - colors      a single color or an array of colors to use for the dimensions
// - valuerange  the y-range of the chart as an array [min, max]
// - heads       an array of gauge charts to render above the submenu section
// - mainheads   an array of gauge charts to render at the menu section

customDashboard.context = {

};
