#!/usr/bin/env sh

cat src/dashboard.js/prologue.js.inc \
    src/dashboard.js/utils.js \
    src/dashboard.js/error-handling.js \
    src/dashboard.js/compatibility.js \
    src/dashboard.js/xss.js \
    src/dashboard.js/colors.js \
    src/dashboard.js/units-conversion.js \
    src/dashboard.js/options.js \
    src/dashboard.js/localstorage.js \
    src/dashboard.js/themes.js \
    src/dashboard.js/charting/dygraph.js \
    src/dashboard.js/charting/sparkline.js \
    src/dashboard.js/charting/google-charts.js \
    src/dashboard.js/charting/gauge.js \
    src/dashboard.js/charting/easy-pie-chart.js \
    src/dashboard.js/charting/d3pie.js \
    src/dashboard.js/charting/d3.js \
    src/dashboard.js/charting/peity.js \
    src/dashboard.js/charting.js \
    src/dashboard.js/common.js \
    src/dashboard.js/main.js \
    src/dashboard.js/alarms.js \
    src/dashboard.js/registry.js \
    src/dashboard.js/boot.js \
    src/dashboard.js/epilogue.js.inc \
    > dashboard.js