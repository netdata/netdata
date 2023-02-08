<!--
title: "`static-threaded` web server"
description: "The Netdata Agent's static-threaded web server spawns a fixed number of threads that listen to web requests and uses non-blocking I/O."
custom_edit_url: https://github.com/netdata/netdata/edit/master/web/server/static/README.md
sidebar_label: "`static-threaded` web server"
learn_status: "Published"
learn_topic_type: "Tasks"
learn_rel_path: "Developers/Web"
-->

# `static-threaded` web server

The `static-threaded` web server spawns a fixed number of threads.
All the threads are concurrently listening for web requests on the same sockets.
The kernel distributes the incoming requests to them.

Each thread uses non-blocking I/O so it can serve any number of web requests in parallel.

This web server respects the `keep-alive` HTTP header to serve multiple HTTP requests via the same connection. 


