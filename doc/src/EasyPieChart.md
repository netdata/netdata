# EasyPieCharts Overview

THIS PAGE IS UNDER CONSTRUCTION

## Settings

### Value Range

You can set the max value of the chart using the following snippet:
```html
<div data-netdata="unique.id"
     data-chart-library="easypiechart"
     data-easypiechart-max-value="40"
     ></div>
```
Be aware that values that exceed the max value will get expanded (e.g. "41" is still 100%). Similar for the minimum:
```html
<div data-netdata="unique.id"
     data-chart-library="easypiechart"
     data-easypiechart-min-value="20"
     ></div>
```
If you specify both minimum and maximum, the rendering behavior changes. Instead of displaying the `value` based from zero, it is now based on the range that is provided by the snippet:
```html
<div data-netdata="unique.id"
     data-chart-library="easypiechart"
     data-easypiechart-min-value="20"
     data-easypiechart-max-value="40"
     ></div>
```
In the first example, a value of `30`, without specifying the minimum, fills the chart bar to `75%` (100% / 40 * 30). However, in this example the range is now `20` (40 - 20 = 20). The value `30` will fill the chart to **`50%`**, since it's in the middle between 20 and 40.

This szenario is useful if you have metrics that change only within a specific range, e.g. temperatures that are very unlikely to fall out of range. In these cases it is more useful to have the chart render the values between the given min and max, to better highlight the changes within them.

### Negative Values

EasyPieCharts can render negative values with the following flag:
```html
<div data-netdata="unique.id"
     data-chart-library="easypiechart"
     data-override-options="signed"
     ></div>
```
Negative values are rendered counter-clockwise.

## Full example

This is a chart that displays the hotwater temperature in the given range of 40 to 50.
```html
<div data-netdata="stiebeleltron_system.hotwater.hotwatertemp"
                     data-title="Hot Water Temperature"
                     data-decimal-digits="1"
                     data-chart-library="easypiechart"
                     data-colors="#FE3912"
                     data-width="55%"
                     data-height="50%"
                     data-points="1200"
                     data-after="-1200"
                     data-dimensions="actual"
                     data-units="°C"
                     data-easypiechart-max-value="50"
                     data-easypiechart-min-value="40"
                     data-common-max="netdata-hotwater-max"
                     data-common-min="netdata-hotwater-min"
></div>
```
![hot water chart](https://user-images.githubusercontent.com/12159026/28666665-a7d68ad2-72c8-11e7-9a96-f6bf9691b471.png)
