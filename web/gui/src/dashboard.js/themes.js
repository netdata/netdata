
NETDATA.themes = {
    white: {
        bootstrap_css: NETDATA.serverStatic + 'css/bootstrap-3.3.7.css',
        dashboard_css: NETDATA.serverStatic + 'dashboard.css?v20180210-1',
        background: '#FFFFFF',
        foreground: '#000000',
        grid: '#F0F0F0',
        axis: '#F0F0F0',
        highlight: '#F5F5F5',
        colors: ['#3366CC', '#DC3912', '#109618', '#FF9900', '#990099', '#DD4477',
            '#3B3EAC', '#66AA00', '#0099C6', '#B82E2E', '#AAAA11', '#5574A6',
            '#994499', '#22AA99', '#6633CC', '#E67300', '#316395', '#8B0707',
            '#329262', '#3B3EAC'],
        easypiechart_track: '#f0f0f0',
        easypiechart_scale: '#dfe0e0',
        gauge_pointer: '#C0C0C0',
        gauge_stroke: '#F0F0F0',
        gauge_gradient: false,
        d3pie: {
            title: '#333333',
            subtitle: '#666666',
            footer: '#888888',
            other: '#aaaaaa',
            mainlabel: '#333333',
            percentage: '#dddddd',
            value: '#aaaa22',
            tooltip_bg: '#000000',
            tooltip_fg: '#efefef',
            segment_stroke: "#ffffff",
            gradient_color: '#000000'
        }
    },
    slate: {
        bootstrap_css: NETDATA.serverStatic + 'css/bootstrap-slate-flat-3.3.7.css?v20161229-1',
        dashboard_css: NETDATA.serverStatic + 'dashboard.slate.css?v20180210-1',
        background: '#272b30',
        foreground: '#C8C8C8',
        grid: '#283236',
        axis: '#283236',
        highlight: '#383838',
        /*          colors: [   '#55bb33', '#ff2222',   '#0099C6', '#faa11b',   '#adbce0', '#DDDD00',
                            '#4178ba', '#f58122',   '#a5cc39', '#f58667',   '#f5ef89', '#cf93c0',
                            '#a5d18a', '#b8539d',   '#3954a3', '#c8a9cf',   '#c7de8a', '#fad20a',
                            '#a6a479', '#a66da8' ],
        */
        colors: ['#66AA00', '#FE3912', '#3366CC', '#D66300', '#0099C6', '#DDDD00',
            '#5054e6', '#EE9911', '#BB44CC', '#e45757', '#ef0aef', '#CC7700',
            '#22AA99', '#109618', '#905bfd', '#f54882', '#4381bf', '#ff3737',
            '#329262', '#3B3EFF'],
        easypiechart_track: '#373b40',
        easypiechart_scale: '#373b40',
        gauge_pointer: '#474b50',
        gauge_stroke: '#373b40',
        gauge_gradient: false,
        d3pie: {
            title: '#C8C8C8',
            subtitle: '#283236',
            footer: '#283236',
            other: '#283236',
            mainlabel: '#C8C8C8',
            percentage: '#dddddd',
            value: '#cccc44',
            tooltip_bg: '#272b30',
            tooltip_fg: '#C8C8C8',
            segment_stroke: "#283236",
            gradient_color: '#000000'
        }
    }
};

if (typeof netdataTheme !== 'undefined' && typeof NETDATA.themes[netdataTheme] !== 'undefined') {
    NETDATA.themes.current = NETDATA.themes[netdataTheme];
} else {
    NETDATA.themes.current = NETDATA.themes.white;
}

NETDATA.colors = NETDATA.themes.current.colors;

// these are the colors Google Charts are using
// we have them here to attempt emulate their look and feel on the other chart libraries
// http://there4.io/2012/05/02/google-chart-color-list/
//NETDATA.colors        = [ '#3366CC', '#DC3912', '#FF9900', '#109618', '#990099', '#3B3EAC', '#0099C6',
//                      '#DD4477', '#66AA00', '#B82E2E', '#316395', '#994499', '#22AA99', '#AAAA11',
//                      '#6633CC', '#E67300', '#8B0707', '#329262', '#5574A6', '#3B3EAC' ];

// an alternative set
// http://www.mulinblog.com/a-color-palette-optimized-for-data-visualization/
//                         (blue)     (red)      (orange)   (green)    (pink)     (brown)    (purple)   (yellow)   (gray)
//NETDATA.colors        = [ '#5DA5DA', '#F15854', '#FAA43A', '#60BD68', '#F17CB0', '#B2912F', '#B276B2', '#DECF3F', '#4D4D4D' ];
