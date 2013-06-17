netdata
=======

linux network traffic web monitoring

This little program will allow you to create embedable live monitoring charts on any web page.

# How it works

1. You run a daemon on your linux: netdata.
This deamon is written in C and is extremely lightweight.
What it does is that it reads /proc/net/dev and for every interface present there, it creates a JSON data file.
This JSON file contains all the data needed for the web graphs.
Since these files are created too often (e.g. once per second) it is adviced to put them on tmpfs.
The files have a fixed length, around just 3k for 60 seconds of graphs.

2. On your web page, you add a few javascript lines and a DIV for every graph you need.
Your browser will hit the web server to fetch the JSON data and refresh the graphs.

3. Graphs are generated using Google Charts API - they are pretty eye candy!



# Installation

### download everything in a directory inside your web docs.
You don't need apache. Any web server able to provide static files will do the job fine.

### Compile netdata
step into the directory you downloaded everything and compile netdata

```sh
gcc -O3 -o netdata netdata.c
```

### run netdata
Run netdata and instruct it to create json files in the current directory.

```sh
./netdata -d -l 60 -u 1 -o `pwd`
```
 - -d says to go daemon mode
 - -l 60 says to show 60 seconds of history
 - -u 1 says to create json files every second
 - -o sets the save path for the json files

You should now have one .json file per interface you have active, in the current directory.
All these files are updated once per second.

### edit index.html
Set in index.html the interfaces you would like to monitor on the web page.

### hit index.html from a web browser
Enjoy real time monitor web graphs for your interfaces.

