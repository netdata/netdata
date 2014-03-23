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
 
 netdata:

  - reads /proc/net/dev and for every interface present there it keeps in its memory a short history of its received and sent bytes.
  - generates JSON HTTP responses containing all the data needed for the web graphs.
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

1. Download everything in a directory inside your web server.
2. cd to that directory
3. run:

```sh
./netdata.start
```

For this to work, you need to have persmission to create a directory in /run or /var/run or /tmp.
If you run into problems, try to become root to verify it works. Please, do not run netdata are root permanently.
Since netdata is a web server, the right thing to do, is to have a special user for it.

Once you run it, the file netdata.conf will be created. You can edit this file to set options for each graph.
To apply the changes you made, you have to run netdata.start again.

To access the panel for all graphs, go to:

 ```
 http://127.0.0.1:19999/
 ```



---

## Installation by hand

### Compile netdata
step into the directory you downloaded everything and compile netdata

```sh
gcc -Wall -O3 -o netdata netdata.c -lpthread
```

### run netdata
Run netdata:

```sh
./netdata -d -l 60 -u 1 -p 19999
```
 - -d says to go daemon mode
 - -l 60 says to keep 60 history entries
 - -u 1 says to add a new history entry every second
 - -p 19999 is the port to listen for web clients

### edit index.html
Set in index.html the interfaces you would like to monitor on the web page.

** REMEMBER: there are 2 sections in index.html to edit. A javascript section and an HTML section. **

### open a web browser

 ```
 http://127.0.0.1:19999/
 ```


