// SPDX-License-Identifier: GPL-3.0+
// ----------------------------------------------------------------------------
// This script periodically updates all the netdata badges you have added to a
// page as images. You don't need this script if you add the badges with
// <embed src="..."/> - embedded badges auto-refresh by themselves.
//
// You can set the following variables before loading this script:

/*global netdata_update_every        *//* number,   the time in seconds to update the badges
 *                                                  (default: 15) */
/*global netdata_live_callback       *//* function, callback to be called on each iteration while updating the badges
 *                                                  (default: null) */
/*global netdata_paused_callback     *//* function, callback to be called when the update pauses
 *                                                  (default: null) */

/*
// EXAMPLE HTML PAGE:

<html>
<head>
<script>
// how frequently to update the badges?
var netdata_update_every = 15;

// show a count-down for badge refreshes
var netdata_live_callback = function(secs, count) {
    document.body.style.opacity = 1;
    if(count)
        document.getElementById("pageliveinfo").innerHTML =  "This page is live - updated <b>" + count + "</b> badges...";
    else
        document.getElementById("pageliveinfo").innerHTML =  "This page is live - badges will be updated in <b>" + secs + "</b> seconds...";
};

// show that we paused refreshes
var netdata_paused_callback = function() {
    document.body.style.opacity = 0.5;
    document.getElementById("pageliveinfo").innerHTML =  "Refresh paused - the page does not have your focus";
};
</script>
<script src="https://localhost:19999/refresh-badges.js"></script>

</head>
<body>
<div id="pageliveinfo">Please wait... loading...</div>
<img src="http://localhost:19999/api/v1/badge.svg?chart=system.cpu"/>
</body>
</html>

*/

if(typeof netdata_update_every === 'undefined')
    netdata_update_every = 15;

var netdata_was_live = false;
var netdata_is_live = true;
var netdata_loops = 0;

function update_netdata_badges() {
    netdata_loops++;
    netdata_is_live = false;

    var updated = 0;
    var focus = document.hasFocus();

    if(focus && netdata_loops >= netdata_update_every) {
        var len = document.images.length;
        while(len--) {
            var url = document.images[len].src;
            if(url.match(/\api\/v1\/badge\.svg/)) {
                if(url.match(/\?/))
                    url = url.replace(/&cacheBuster=\d*/, "") + "&cacheBuster=" + new Date().getTime().toString();
                else
                    url = url.replace(/\?cacheBuster=\d*/, "") + "?cacheBuster=" + new Date().getTime().toString();

                document.images[len].src = url;
                updated++;
            }
        }
        netdata_loops = 0;
    }

    if(focus || updated)
        netdata_is_live = true;

    try {
        if(netdata_is_live && typeof netdata_live_callback === 'function')
            netdata_live_callback(netdata_update_every - netdata_loops, updated);
        else if(netdata_was_live !== netdata_is_live && typeof netdata_paused_callback === 'function')
            netdata_paused_callback();
    }
    catch(e) {
        console.log(e);
    }
    netdata_was_live = netdata_is_live;

    setTimeout(update_netdata_badges, 1000);
}
setTimeout(update_netdata_badges, 1000);
