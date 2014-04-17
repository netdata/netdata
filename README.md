netdata
=======

linux real time system monitoring

This program is a daemon that collects system information from /proc and other sources.
It is heavily optimized and lightweight.

It updates everything every second!
But tt only needs a few microseconds (just a fraction of a millisecond) of one of your cores and a few megabytes of memory.

If listens on port 19999 and it will give you a full featured web interface using google charts.

Check it live at:

http://www.tsaousis.gr:19999/

Here is a screenshot:

![image](https://cloud.githubusercontent.com/assets/2662304/2593406/3c797e88-ba80-11e3-8ec7-c10174d59ad6.png)

# How it works

1. You run a daemon on your linux: netdata.
 This deamon is written in C and is extremely lightweight.
 
 netdata:

  - reads several /proc files
  - keeps track of the values in memroy (a short history)
  - generates JSON and JSONP HTTP responses containing all the data needed for the web graphs
  - is a web server. You can access JSON data by using:
 
 ```
 http://127.0.0.1:19999/data/net.eth0
 ```
 
 This will give you the JSON file for traffic on eth0.
 The above is equivalent to:
 
 ```
 http://127.0.0.1:19999/data/net.eth0/3600/1/average
 ```
 
 where:

  - 3600 is the number of entries to generate (3600 is a default which can be overwritten by -l).
  - 1 is grouping count, 1 = every single entry, 2 = half the entries, 3 = one every 3 entries, etc
  - `average` is the grouping method. It can also be `max`.


2. On your web page, you add a few javascript lines and a DIV for every graph you need.
 Your browser will hit the web server to fetch the JSON data and refresh the graphs.

3. Graphs are generated using Google Charts API.



# Installation

## Automatic installation

1. Download the git.
2. cd to that directory
3. run:

```sh
./netdata.start
```

Once you run it, the file netdata.conf will be created. You can edit this file to set options for each graph.
To apply the changes you made, you have to run netdata.start again.

To access the web site for all graphs, go to:

 ```
 http://127.0.0.1:19999/
 ```



---

## Installation by hand

### Compile netdata
step into the directory you downloaded everything and compile netdata

```sh
gcc -Wall -O3 -o netdata netdata.c -lpthread -lz
```

### run netdata
Run netdata:

```sh
./netdata -u nobody -d -l 1200 -u 1 -p 19999
```
 - -d says to go daemon mode
 - -l 1200 says to keep 1200 history entries (20 minutes)
 - -t 1 says to add a new history entry every second
 - -p 19999 is the port to listen for web clients
 - -u nobody to run as nobody

### open a web browser

 ```
 http://127.0.0.1:19999/
 ```

