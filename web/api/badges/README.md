# Netdata badges

**Badges are cool!**

Netdata can generate badges for any chart and any dimension at any time-frame. Badges come in `SVG` and can be added to any web page using an `<IMG>` HTML tag.

**Netdata badges are powerful**!

Given that netdata collects from **1.000** to **5.000** metrics per server (depending on the number of network interfaces, disks, cpu cores, applications running, users logged in, containers running, etc) and that netdata already has data reduction/aggregation functions embedded, the badges can be quite powerful.

For each metric/dimension and for arbitrary time-frames badges can show **min**, **max** or **average** value, but also **sum** or **incremental-sum** to have their **volume**.

For example, there is [a chart in netdata that shows the current requests/s of nginx](http://london.my-netdata.io/#nginx_local_nginx). Using this chart alone we can show the following badges (we could add more time-frames, like **today**, **yesterday**, etc):

<a href="https://registry.my-netdata.io/#nginx_local_nginx"><img src="https://registry.my-netdata.io/api/v1/badge.svg?chart=nginx_local.connections&dimensions=active&value_color=grey:null%7Cblue&label=nginx%20active%20connections%20now&units=null&precision=0"/></a>  <a href="https://registry.my-netdata.io/#nginx_local_nginx"><img src="https://registry.my-netdata.io/api/v1/badge.svg?chart=nginx_local.connections&dimensions=active&after=-3600&value_color=orange&label=last%20hour%20average&units=null&options=unaligned&precision=0"/></a> <a href="https://registry.my-netdata.io/#nginx_local_nginx"><img src="https://registry.my-netdata.io/api/v1/badge.svg?chart=nginx_local.connections&dimensions=active&group=max&after=-3600&value_color=red&label=last%20hour%20max&units=null&options=unaligned&precision=0"/></a>

Similarly, there is [a chart that shows outbound bandwidth per class](http://london.my-netdata.io/#tc_eth0), using QoS data. So it shows `kilobits/s` per class. Using this chart we can show:

<a href="https://registry.my-netdata.io/#tc_eth0"><img src="https://registry.my-netdata.io/api/v1/badge.svg?chart=tc.world_out&dimensions=web_server&value_color=green&label=web%20server%20sends%20now&units=kbps"/></a> <a href="https://registry.my-netdata.io/#tc_eth0"><img src="https://registry.my-netdata.io/api/v1/badge.svg?chart=tc.world_out&dimensions=web_server&after=-86400&options=unaligned&group=sum&divide=8388608&value_color=blue&label=web%20server%20sent%20today&units=GB"/></a>

The right one is a **volume** calculation. Netdata calculated the total of the last 86.400 seconds (a day) which gives `kilobits`, then divided it by 8 to make it KB, then by 1024 to make it MB and then by 1024 to make it GB. Calculations like this are quite accurate, since for every value collected, every second, netdata interpolates it to second boundary using microsecond calculations.

Let's see a few more badge examples (they come from the [netdata registry](../../../registry/)):

- **cpu usage of user `root`** (you can pick any user; 100% = 1 core). This will be `green <10%`, `yellow <20%`, `orange <50%`, `blue <100%` (1 core), `red` otherwise (you define thresholds and colors on the URL).

  <a href="https://registry.my-netdata.io/#apps_cpu"><img src="https://registry.my-netdata.io/api/v1/badge.svg?chart=users.cpu&dimensions=root&value_color=grey:null%7Cgreen%3C10%7Cyellow%3C20%7Corange%3C50%7Cblue%3C100%7Cred&label=root%20user%20cpu%20now&units=%25"></img></a> <a href="https://registry.my-netdata.io/#apps_cpu"><img src="https://registry.my-netdata.io/api/v1/badge.svg?chart=users.cpu&dimensions=root&after=-3600&value_color=grey:null%7Cgreen%3C10%7Cyellow%3C20%7Corange%3C50%7Cblue%3C100%7Cred&label=root%20user%20average%20cpu%20last%20hour&units=%25"></img></a>

- **mysql queries per second**

  <a href="https://registry.my-netdata.io/#mysql_local"><img src="https://registry.my-netdata.io/api/v1/badge.svg?chart=mysql_local.queries&dimensions=questions&label=mysql%20queries%20now&value_color=red&units=%5Cs"></img></a> <a href="https://registry.my-netdata.io/#mysql_local"><img src="https://registry.my-netdata.io/api/v1/badge.svg?chart=mysql_local.queries&dimensions=questions&after=-3600&options=unaligned&group=sum&label=mysql%20queries%20this%20hour&value_color=green&units=null"></img></a> <a href="https://registry.my-netdata.io/#mysql_local"><img src="https://registry.my-netdata.io/api/v1/badge.svg?chart=mysql_local.queries&dimensions=questions&after=-86400&options=unaligned&group=sum&label=mysql%20queries%20today&value_color=blue&units=null"></img></a>

  niche ones: **mysql SELECT statements with JOIN, which did full table scans**:

  <a href="https://registry.my-netdata.io/#mysql_local_issues"><img src="https://registry.my-netdata.io/api/v1/badge.svg?chart=mysql_local.join_issues&dimensions=scan&after=-3600&label=full%20table%20scans%20the%20last%20hour&value_color=orange&group=sum&units=null"></img></a>

---

> So, every single line on the charts of a [netdata dashboard](http://london.my-netdata.io/), can become a badge and this badge can calculate **average**, **min**, **max**, or **volume** for any time-frame! And you can also vary the badge color using conditions on the calculated value.

---

## How to create badges

The basic URL is `http://your.netdata:19999/api/v1/badge.svg?option1&option2&option3&...`.

Here is what you can put for `options` (these are standard netdata API options):

- `chart=CHART.NAME`

  The chart to get the values from.

  **This is the only parameter required** and with just this parameter, netdata will return the sum of the latest values of all chart dimensions.

  Example:

```html
  <a href="#">
     <img src="https://registry.my-netdata.io/api/v1/badge.svg?chart=system.cpu"></img>
  </a>
```

  Which produces this:

  <a href="#">
     <img src="https://registry.my-netdata.io/api/v1/badge.svg?chart=system.cpu"></img>
  </a>

- `alarm=NAME`

  Render the current value and status of an alarm linked to the chart. This option can be ignored if the badge to be generated is not related to an alarm.

  The current value of the alarm will be rendered. The color of the badge will indicate the status of the alarm.

  For alarm badges, **both `chart` and `alarm` parameters are required**.

- `dimensions=DIMENSION1|DIMENSION2|...`

  The dimensions of the chart to use. If you don't set any dimension, all will be used. When multiple dimensions are used, netdata will sum their values. You can append `options=absolute` if you want this sum to convert all values to positive before adding them.

  Pipes in HTML have to escaped with `%7C`.

  Example:

```html
  <a href="#">
     <img src="https://registry.my-netdata.io/api/v1/badge.svg?chart=system.cpu&dimensions=system%7Cnice"></img>
  </a>
```

  Which produces this:

  <a href="#">
     <img src="https://registry.my-netdata.io/api/v1/badge.svg?chart=system.cpu&dimensions=system%7Cnice"></img>
  </a>

- `before=SECONDS` and `after=SECONDS`

  The timeframe. These can be absolute unix timestamps, or relative to now, number of seconds. By default `before=0` and `after=-1` (1 second in the past).

  To get the last minute set `after=-60`. This will give the average of the last complete minute (XX:XX:00 - XX:XX:59).

  To get the max of the last hour set `after=-3600&group=max`. This will give the maximum value of the last complete hour (XX:00:00 - XX:59:59)

  Example:

```html
  <a href="#">
     <img src="https://registry.my-netdata.io/api/v1/badge.svg?chart=system.cpu&after=-60"></img>
  </a>
```

  Which produces the average of last complete minute (XX:XX:00 - XX:XX:59):

  <a href="#">
     <img src="https://registry.my-netdata.io/api/v1/badge.svg?chart=system.cpu&after=-60"></img>
  </a>

  While this is the previous minute (one minute before the last one, again aligned XX:XX:00 - XX:XX:59):

```html
  <a href="#">
     <img src="https://registry.my-netdata.io/api/v1/badge.svg?chart=system.cpu&before=-60&after=-60"></img>
  </a>
```

  It produces this:
  
  <a href="#">
     <img src="https://registry.my-netdata.io/api/v1/badge.svg?chart=system.cpu&before=-60&after=-60"></img>
  </a>

- `group=min` or `group=max` or `group=average` (the default) or `group=sum` or `group=incremental-sum`

  If netdata will have to reduce (aggregate) the data to calculate the value, which aggregation method to use.

  - `max` will find the max value for the timeframe. This works on both positive and negative dimensions. It will find the most extreme value.

  - `min` will find the min value for the timeframe. This works on both positive and negative dimensions. It will find the number closest to zero.

  - `average` will calculate the average value for the timeframe.

  - `sum` will sum all the values for the timeframe. This is nice for finding the volume of dimensions for a timeframe. So if you have a dimension that reports `X per second`, you can find the volume of the dimension in a timeframe, by adding its values in that timeframe.

  - `incremental-sum` will sum the difference of each value to its next. Let's assume you have a dimension that does not measure the rate of something, but the absolute value of it. So it has values like this "1, 5, 3, 7, 4". `incremental-sum` will calculate the difference of adjacent values. In this example, they will be `(5 - 1) + (3 - 5) + (7 - 3) + (4 - 7) = 3` (which is equal to the last value minus the first = 4 - 1).

- `options=opt1|opt2|opt3|...`

  These fine tune various options of the API. Here is what you can use for badges (the API has more option, but only these are useful for badges):

  - `percentage`, instead of returning the value, calculate the percentage of the sum of the selected dimensions, versus the sum of all the dimensions of the chart. This also sets the units to `%`.

  - `absolute` or `abs`, turn all values positive and then sum them.

  - `display_absolute` or `display-absolute`, to use the signed value during color calculation, but display the absolute value on the badge.

  - `min2max`, when multiple dimensions are given, do not sum them, but take their `max - min`.

  - `unaligned`, when data are reduced / aggregated (e.g. the request is about the average of the last minute, or hour), netdata by default aligns them so that the charts will have a constant shape (so average per minute returns always XX:XX:00 - XX:XX:59). Setting the `unaligned` option, netdata will aggregate data without any alignment, so if the request is for 60 seconds, it will aggregate the latest 60 seconds of collected data.

These are options dedicated to badges:

- `label=TEXT`

  The label of the badge.

- `units=TEXT`

  The units of the badge. If you want to put a `/`, please put a `\`. This is because netdata allows badges parameters to be given as path in URL, instead of query string. You can also use `null` or `empty` to show it without any units.

  The units `seconds`, `minutes` and `hours` trigger special formatting. The value has to be in this unit, and netdata will automatically change it to show a more pretty duration.

- `multiply=NUMBER`

  Multiply the value with this number. The default is `1`.

- `divide=NUMBER`

  Divide the value with this number. The default is `1`.

- `label_color=COLOR`

  The color of the label (the left part). You can use any HTML color, include `#NNN` and `#NNNNNN`. The following colors are defined in netdata (and you can use them by name): `green`, `brightgreen`, `yellow`, `yellowgreen`, `orange`, `red`, `blue`, `grey`, `gray`, `lightgrey`, `lightgray`. These are taken from https://github.com/badges/shields so they are compatible with standard badges.

- `value_color=COLOR:null|COLOR<VALUE|COLOR>VALUE|COLOR>=VALUE|COLOR<=VALUE|...`

  You can add a pipe delimited list of conditions to pick the color. The first matching (left to right) will be used.

  Example: `value_color=grey:null|green<10|yellow<100|orange<1000|blue<10000|red`

  The above will set `grey` if no value exists (not collected within the `gap when lost iterations above` in netdata.conf for the chart), `green` if the value is less than 10, `yellow` if the value is less than 100, etc up to `red` which will be used if no other conditions match.

  The supported operators are `<`, `>`, `<=`, `>=`, `=` (or `:`) and `!=` (or `<>`).

- `precision=NUMBER`

  The number of decimal digits of the value. By default netdata will add:

  - no decimal digits for values > 1000
  - 1 decimal digit for values > 100
  - 2 decimal digits for values > 1
  - 3 decimal digits for values > 0.1
  - 4 decimal digits for values <= 0.1

  Using the `precision=NUMBER` you can set your preference per badge.

- `scale=XXX`

  This option scales the svg image. It accepts values above or equal to 100 (100% is the default scale). For example, lets get a few different sizes:

     <img src="https://registry.my-netdata.io/api/v1/badge.svg?chart=system.cpu&after=-60&scale=100"></img> original<br/>
     <img src="https://registry.my-netdata.io/api/v1/badge.svg?chart=system.cpu&after=-60&scale=125"></img> `scale=125`<br/>
     <img src="https://registry.my-netdata.io/api/v1/badge.svg?chart=system.cpu&after=-60&scale=150"></img> `scale=150`<br/>
     <img src="https://registry.my-netdata.io/api/v1/badge.svg?chart=system.cpu&after=-60&scale=175"></img> `scale=175`<br/>
     <img src="https://registry.my-netdata.io/api/v1/badge.svg?chart=system.cpu&after=-60&scale=200"></img> `scale=200`


- `refresh=auto` or `refresh=SECONDS`

  This option enables auto-refreshing of images. netdata will send the HTTP header `Refresh: SECONDS` to the web browser, thus requesting automatic refresh of the images at regular intervals.

  `auto` will calculate the proper `SECONDS` to avoid unnecessary refreshes. If `SECONDS` is zero, this feature is disabled (it is also disabled by default).

  Auto-refreshing like this, works only if you access the badge directly. So, you may have to put it an `embed` or `iframe` for it to be auto-refreshed. Use something like this:

```html
<embed src="BADGE_URL" type="image/svg+xml" height="20" />
```

  Another way is to use javascript to auto-refresh them. You can auto-refresh all the netdata badges on a page using javascript. You have to add a class to all the netdata badges, like this `<img class="netdata-badge" src="..."/>`. Then add this javascript code to your page (it requires jquery):

```html
<script>
    var NETDATA_BADGES_AUTOREFRESH_SECONDS = 5;
    function refreshNetdataBadges() {
      var now = new Date().getTime().toString();
      $('.netdata-badge').each(function() {
        this.src = this.src.replace(/\&_=\d*/, '') + '&_=' + now;
      });
      setTimeout(refreshNetdataBadges, NETDATA_BADGES_AUTOREFRESH_SECONDS * 1000);
    }
    setTimeout(refreshNetdataBadges, NETDATA_BADGES_AUTOREFRESH_SECONDS * 1000);
</script>
```

A more advanced badges refresh method is to include `http://your.netdata.ip:19999/refresh-badges.js` in your page. For more information and use example, [check this](https://github.com/netdata/netdata/blob/master/web/gui/refresh-badges.js).

---

## Escaping URLs

Keep in mind that if you add badge URLs to your HTML pages you have to escape the special characters:

character|name|escape sequence
:-------:|:--:|:-------------:
`   `|space (in labels and units)|`%20`
` # `|hash (for colors)|`%23`
` % `|percent (in units)|`%25`
` < `|less than|`%3C`
` > `|greater than|`%3E`
` \ `|backslash (when you need a `/`)|`%5C`
` \| `|pipe (delimiting parameters)|`%7C`

---

## Using the path instead of the query string

The badges can also be generated using the URL path for passing parameters. The format is exactly the same.

So instead of:

  `http://your.netdata:19999/api/v1/badge.svg?option1&option2&option3&...`

you can write:

  `http://your.netdata:19999/api/v1/badge.svg/option1/option2/option3/...`

You can also append anything else you like, like this:

  `http://your.netdata:19999/api/v1/badge.svg/option1/option2/option3/my-super-badge.svg`

## FAQ

#### Is it fast?
On modern hardware, netdata can generate about **2.000 badges per second per core**, before noticing any delays. It generates a badge in about half a millisecond!

Of course these timing are for badges that use recent data. If you need badges that do calculations over long durations (a day, or more), timing will differ. netdata logs its timings at its `access.log`, so take a look there before adding a heavy badge on a busy web site. Of course, you can cache such badges or have a cron job get them from netdata and save them at your web server at regular intervals.


#### Embedding badges in github

You have 2 options a) SVG images with markdown and b) SVG images with HTML (directly in .md files).

For example, this is the cpu badge shown above:

- Markdown example:

```md
[![A nice name](https://registry.my-netdata.io/api/v1/badge.svg?chart=users.cpu&dimensions=root&value_color=grey:null%7Cgreen%3C10%7Cyellow%3C20%7Corange%3C50%7Cblue%3C100%7Cred&label=root%20user%20cpu%20now&units=%25)](https://registry.my-netdata.io/#apps_cpu)
```

- HTML example:

```html
<a href="https://registry.my-netdata.io/#apps_cpu">
    <img src="https://registry.my-netdata.io/api/v1/badge.svg?chart=users.cpu&dimensions=root&value_color=grey:null%7Cgreen%3C10%7Cyellow%3C20%7Corange%3C50%7Cblue%3C100%7Cred&label=root%20user%20cpu%20now&units=%25"></img>
</a>
```

Both produce this:

<a href="https://registry.my-netdata.io/#apps_cpu">
    <img src="https://registry.my-netdata.io/api/v1/badge.svg?chart=users.cpu&dimensions=root&value_color=grey:null%7Cgreen%3C10%7Cyellow%3C20%7Corange%3C50%7Cblue%3C100%7Cred&label=root%20user%20cpu%20now&units=%25"></img>
</a>

#### auto-refreshing badges in github

Unfortunately it cannot be done. Github fetches all the images using a proxy and rewrites all the URLs to be served by the proxy.

You can refresh them from your browser console though. Press F12 to open the web browser console (switch to the console too), paste the following and press enter. They will refresh:

```js
var len = document.images.length; while(len--) { document.images[len].src = document.images[len].src.replace(/\?cacheBuster=\d*/, "") + "?cacheBuster=" + new Date().getTime().toString(); };
```
