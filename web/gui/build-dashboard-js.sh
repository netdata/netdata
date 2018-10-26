#!/usr/bin/env sh

cat src/prologue.js.inc \
    src/utils.js \
    src/error-handling.js \
    src/compatibility.js \
    src/xss.js \
    src/colors.js \
    src/units-conversion.js \
    src/options.js \
    src/localstorage.js \
    src/charting/dygraph.js \
    src/charting/sparkline.js \
    src/charting/google-charts.js \
    src/charting/gauge.js \
    src/charting/easy-pie-chart.js \
    src/charting/d3pie.js \
    src/charting/d3.js \
    src/charting/peity.js \
    src/main.js \
    src/alarms.js \
    src/registry.js \
    src/boot.js \
    src/epilogue.js.inc \
    > dashboard.js
