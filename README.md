netdata
=======

linux network traffic web monitoring

This little program will allow you to create embedable live monitoring charts on any web page.

Example server with 6 interfaces:
![image](https://f.cloud.github.com/assets/2662304/664777/3dad6c32-d78d-11e2-9ecf-b921afebfb0b.png)

Example server with 20 interfaces:
![image](https://f.cloud.github.com/assets/2662304/689979/807dfb4e-dac6-11e2-8546-6d83fdb05866.png)


# How it works

1. You run a daemon on your linux: netdata.
This deamon is written in C and is extremely lightweight.
What it does is that it reads /proc/net/dev and for every interface present there, it creates a JSON data file.
This JSON file contains all the data needed for the web graphs.
Since these files are created too often (e.g. once per second) it is adviced to put them on tmpfs.
The files have a fixed length, around just 3k for 60 seconds of graphs.

2. On your web page, you add a few javascript lines and a DIV for every graph you need.
Your browser will hit the web server to fetch the JSON data and refresh the graphs.

3. Graphs are generated using Google Charts API.



# Installation

## Automatic installation

1. Download everything in a directory inside your web server.
2. cd to that directory
3. run:

```sh
./netdata.start
```

For this to work, you need to have persmission to create a directory in /run or /var/run or /tmp.
If you run into problems, try to become root to verify it works.

Once you run it, the file netdata.conf will be created. You can edit this file to set options for each graph.
To apply the changes you made, you have to run netdata.start again.

The directory /run/netdata (or /var/run/netdata, or /tmp/netdata depending on your distro) is ready to be
served by a web server (it contains all files needed). Just link it to your htdocs and access it from your web browser.

---

## Installation by hand

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
./netdata -d -l 60 -u 1 -o "`pwd`"
```
 - -d says to go daemon mode
 - -l 60 says to show 60 seconds of history
 - -u 1 says to create json files every second
 - -o sets the save path for the json files

You should now have one .json file per interface you have active, in the current directory.
All these files are updated once per second.

### edit index.html
Set in index.html the interfaces you would like to monitor on the web page.

** REMEMBER: there are 2 sections in index.html to edit. A javascript section and an HTML section. **


### hit index.html from a web browser
Enjoy real time monitor web graphs for your interfaces.

