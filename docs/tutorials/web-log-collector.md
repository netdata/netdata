# Monitor web server log files with Netdata

Web server log files 

By parsing web server log files with Netdata, you 


    too many redirects (i.e. oops! this should not redirect clients to itself)
    too many bad requests (i.e. oops! a few files were not uploaded)
    too many internal server errors (i.e. oops! this release crashes too much)
    unreasonably too many requests (i.e. oops! we are under attack)
    unreasonably few requests (i.e. oops! call the network guys)
    unreasonably slow responses (i.e. oops! the database is slow again)
    too few successful responses (i.e. oops! help us God!)


Netdata has been capable of monitoring web log files for quite some time, thanks for the [weblog python.d
module](../../collectors/python.d.plugin/web_log/README.md), but this Go-based refactoring offers a ton of improvements.

This tutorial will walk you through using the new Go-based web log collector to turn these 



## Set up your web servers


## Configure the web_log collector module


## Tweak web_log collector alarms


## What's next?

