import { Attributes } from "./transformDataAttributes"

export type ChartLibraryName = "dygraph" | "sparkline"
// | "peity" | "google" | "d3pie" | "d3"
  | "easypiechart" | "gauge"
// | "textonly"
export interface ChartLibraryConfig {
  aspectRatio?: number
  format: string
  hasLegend: (attributes: Attributes) => boolean
  hasToolboxPanAndZoom: boolean
  isLogScale?: (attributes: Attributes) => boolean
  options: (attributes: Attributes) => string
  trackColors: boolean
  pixelsPerPoint: (attributes: Attributes) => number
  xssRegexIgnore: RegExp
  containerClass: (attributes: Attributes) => string
}
export type ChartLibrariesSettings = {
  [key in ChartLibraryName]: ChartLibraryConfig
}

type IsDygraphSparkline = (attributes: Attributes) => boolean
const isDygraphSparkline: IsDygraphSparkline = (attributes) => (
  attributes.dygraphTheme === "sparkline"
)

export const chartLibrariesSettings: ChartLibrariesSettings = {
  dygraph: {
    // initialize: window.NETDATA.dygraphInitialize,
    // create: window.NETDATA.dygraphChartCreate,
    // update: window.NETDATA.dygraphChartUpdate,
    // resize(state) {
    //   if (typeof state.tmp.dygraph_instance !== "undefined"
    //     && typeof state.tmp.dygraph_instance.resize === "function") {
    //     state.tmp.dygraph_instance.resize()
    //   }
    // },
    // setSelection: window.NETDATA.dygraphSetSelection,
    // clearSelection: window.NETDATA.dygraphClearSelection,
    hasToolboxPanAndZoom: true,
    // initialized: false,
    // enabled: true,
    xssRegexIgnore: new RegExp("^/api/v1/data.result.data$"),
    format: "json",
    options(attributes: Attributes) {
      if (typeof this.isLogScale === "function") {
        // flip - in proper order (from oldest to newest)
        return `ms|flip${this.isLogScale(attributes) ? "|abs" : ""}`
      }
      return ""
    },
    hasLegend(attributes: Attributes) {
      // not using __hasLegendCache__ as in old-dashboard, because performance tweaks like this
      // probably won't be needed in react app
      return !isDygraphSparkline(attributes) && attributes.legend
    },
    // autoresize(state) {
    //   void (state)
    //   return true
    // },
    // max_updates_to_recreate(state) {
    //   void (state)
    //   return 5000
    // },
    trackColors: true,
    pixelsPerPoint: ((attributes: Attributes) => (isDygraphSparkline(attributes) ? 2 : 3)),
    // pixels_per_point(state) {
    //   return (this.isSparkline(state) === false) ? 3 : 2
    // },
    isLogScale(attributes: Attributes) {
      return attributes.dygraphTheme === "logscale"
    },
    containerClass(attributes: Attributes) {
      return this.hasLegend(attributes)
        ? "netdata-container-with-legend"
        : "netdata-container"
    },
    // container_class(state) {
    //   if (this.legend(state) !== null) {
    //     return "netdata-container-with-legend"
    //   }
    //   return "netdata-container"
    // },
  },
  sparkline: {
  //   initialize: window.NETDATA.sparklineInitialize,
  //   create: window.NETDATA.sparklineChartCreate,
  //   update: window.NETDATA.sparklineChartUpdate,
  //   resize: null,
  //   setSelection: undefined, // function(state, t) { void(state); return true; },
  //   clearSelection: undefined, // function(state) { void(state); return true; },
    hasToolboxPanAndZoom: false,
  //   initialized: false,
  //   enabled: true,
    xssRegexIgnore: new RegExp("^/api/v1/data.result$"),
    format: "array",
    options: () => "flip|abs",
    hasLegend: () => false,
  //   autoresize(state) {
  //     void (state)
  //     return false
  //   },
  //   max_updates_to_recreate(state) {
  //     void (state)
  //     return 5000
  //   },
    trackColors: false,
    pixelsPerPoint: () => 3,
    containerClass: () => "netdata-container",
  },
  // peity: {
  //   initialize: window.NETDATA.peityInitialize,
  //   create: window.NETDATA.peityChartCreate,
  //   update: window.NETDATA.peityChartUpdate,
  //   resize: null,
  //   setSelection: undefined, // function(state, t) { void(state); return true; },
  //   clearSelection: undefined, // function(state) { void(state); return true; },
  //   toolboxPanAndZoom: null,
  //   initialized: false,
  //   enabled: true,
  //   xssRegexIgnore: new RegExp("^/api/v1/data\.result$"),
  //   format(state) {
  //     void (state)
  //     return "ssvcomma"
  //   },
  //   options(state) {
  //     void (state)
  //     return "null2zero" + "%7C" + "flip" + "%7C" + "abs"
  //   },
  //   legend(state) {
  //     void (state)
  //     return null
  //   },
  //   autoresize(state) {
  //     void (state)
  //     return false
  //   },
  //   max_updates_to_recreate(state) {
  //     void (state)
  //     return 5000
  //   },
  //   track_colors(state) {
  //     void (state)
  //     return false
  //   },
  //   pixels_per_point(state) {
  //     void (state)
  //     return 3
  //   },
  //   container_class(state) {
  //     void (state)
  //     return "netdata-container"
  //   },
  // },
  // google: {
  //   initialize: window.NETDATA.googleInitialize,
  //   create: window.NETDATA.googleChartCreate,
  //   update: window.NETDATA.googleChartUpdate,
  //   resize: null,
  //   setSelection: undefined, // function(state, t) { void(state); return true; },
  //   clearSelection: undefined, // function(state) { void(state); return true; },
  //   toolboxPanAndZoom: null,
  //   initialized: false,
  //   enabled: true,
  //   xssRegexIgnore: new RegExp("^/api/v1/data\.result.rows$"),
  //   format(state) {
  //     void (state)
  //     return "datatable"
  //   },
  //   options(state) {
  //     void (state)
  //     return ""
  //   },
  //   legend(state) {
  //     void (state)
  //     return null
  //   },
  //   autoresize(state) {
  //     void (state)
  //     return false
  //   },
  //   max_updates_to_recreate(state) {
  //     void (state)
  //     return 300
  //   },
  //   track_colors(state) {
  //     void (state)
  //     return false
  //   },
  //   pixels_per_point(state) {
  //     void (state)
  //     return 4
  //   },
  //   container_class(state) {
  //     void (state)
  //     return "netdata-container"
  //   },
  // },
  // d3pie: {
  //   initialize: window.NETDATA.d3pieInitialize,
  //   create: window.NETDATA.d3pieChartCreate,
  //   update: window.NETDATA.d3pieChartUpdate,
  //   resize: null,
  //   setSelection: window.NETDATA.d3pieSetSelection,
  //   clearSelection: window.NETDATA.d3pieClearSelection,
  //   toolboxPanAndZoom: null,
  //   initialized: false,
  //   enabled: true,
  //   xssRegexIgnore: new RegExp("^/api/v1/data\.result.data$"),
  //   format(state) {
  //     void (state)
  //     return "json"
  //   },
  //   options(state) {
  //     void (state)
  //     return "objectrows" + "%7C" + "ms"
  //   },
  //   legend(state) {
  //     void (state)
  //     return null
  //   },
  //   autoresize(state) {
  //     void (state)
  //     return false
  //   },
  //   max_updates_to_recreate(state) {
  //     void (state)
  //     return 5000
  //   },
  //   track_colors(state) {
  //     void (state)
  //     return false
  //   },
  //   pixels_per_point(state) {
  //     void (state)
  //     return 15
  //   },
  //   container_class(state) {
  //     void (state)
  //     return "netdata-container"
  //   },
  // },
  // d3: {
  //   initialize: window.NETDATA.d3Initialize,
  //   create: window.NETDATA.d3ChartCreate,
  //   update: window.NETDATA.d3ChartUpdate,
  //   resize: null,
  //   setSelection: undefined, // function(state, t) { void(state); return true; },
  //   clearSelection: undefined, // function(state) { void(state); return true; },
  //   toolboxPanAndZoom: null,
  //   initialized: false,
  //   enabled: true,
  //   xssRegexIgnore: new RegExp("^/api/v1/data\.result.data$"),
  //   format(state) {
  //     void (state)
  //     return "json"
  //   },
  //   options(state) {
  //     void (state)
  //     return ""
  //   },
  //   legend(state) {
  //     void (state)
  //     return null
  //   },
  //   autoresize(state) {
  //     void (state)
  //     return false
  //   },
  //   max_updates_to_recreate(state) {
  //     void (state)
  //     return 5000
  //   },
  //   track_colors(state) {
  //     void (state)
  //     return false
  //   },
  //   pixels_per_point(state) {
  //     void (state)
  //     return 3
  //   },
  //   container_class(state) {
  //     void (state)
  //     return "netdata-container"
  //   },
  // },
  easypiechart: {
  //   initialize: window.NETDATA.easypiechartInitialize,
  //   create: window.NETDATA.easypiechartChartCreate,
  //   update: window.NETDATA.easypiechartChartUpdate,
  //   resize: null,
  //   setSelection: window.NETDATA.easypiechartSetSelection,
  //   clearSelection: window.NETDATA.easypiechartClearSelection,
    hasToolboxPanAndZoom: false,
    //   initialized: false,
    //   enabled: true,
    xssRegexIgnore: new RegExp("^/api/v1/data.result$"),
    format: "array",
    options() {
      return "absolute"
    },
    hasLegend() {
      return false
    },
    //   autoresize(state) {
    //     void (state)
    //     return false
    //   },
    //   max_updates_to_recreate(state) {
    //     void (state)
    //     return 5000
    //   },
    trackColors: true,
    pixelsPerPoint: () => 3,
    aspectRatio: 100,
    containerClass: () => "netdata-container-easypiechart",
  },
  gauge: {
    // initialize: window.NETDATA.gaugeInitialize,
    // create: window.NETDATA.gaugeChartCreate,
    // update: window.NETDATA.gaugeChartUpdate,
    // resize: null,
    // setSelection: window.NETDATA.gaugeSetSelection,
    // clearSelection: window.NETDATA.gaugeClearSelection,
    hasToolboxPanAndZoom: false,
    // initialized: false,
    // enabled: true,
    xssRegexIgnore: new RegExp("^/api/v1/data.result$"),
    format: "array",
    options: () => "absolute",
    hasLegend: () => false,
    // autoresize(state) {
    //   void (state)
    //   return false
    // },
    // max_updates_to_recreate(state) {
    //   void (state)
    //   return 5000
    // },
    trackColors: true,
    pixelsPerPoint: () => 3,
    aspectRatio: 60,
    containerClass: () => "netdata-container-gauge",
  },
  // textonly: {
  //   autoresize(state) {
  //     void (state)
  //     return false
  //   },
  //   container_class(state) {
  //     void (state)
  //     return "netdata-container"
  //   },
  //   create: window.NETDATA.textOnlyCreate,
  //   enabled: true,
  //   format(state) {
  //     void (state)
  //     return "array"
  //   },
  //   initialized: true,
  //   initialize(callback) {
  //     callback()
  //   },
  //   legend(state) {
  //     void (state)
  //     return null
  //   },
  //   max_updates_to_recreate(state) {
  //     void (state)
  //     return 5000
  //   },
  //   options(state) {
  //     void (state)
  //     return "absolute"
  //   },
  //   pixels_per_point(state) {
  //     void (state)
  //     return 3
  //   },
  //   track_colors(state) {
  //     void (state)
  //     return false
  //   },
  //   update: window.NETDATA.textOnlyUpdate,
  //   xssRegexIgnore: new RegExp("^/api/v1/data\.result$"),
  // },
}
