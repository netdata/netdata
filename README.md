netdata
=======

linux network traffic web monitoring


# Installation

1. download everything in a directory inside your web docs. You don't need apache. Any web server able to provide static files will do the job fine.

2. step into this directory and compile netdata

```sh
gcc -O3 -o netdata netdata.c
```

3. run netdata and instruct it to create json files in this directory

```sh
./netdata -d -l 60 -u 1 -o `pwd`
```
-d says to go daemon mode
-l 60 says to show 60 seconds of history
-u 1 says to create json files every second
-o sets the save path for the json files

You should now have one .json file per interface you have active, in the current directory.
All these files are updated once per second.

4. edit index.html and set the interfaces you would like to monitor on the web page.

5. hit index.html from a web browser, through your web server.

6. Enjoy real time monitor web graphs for your interfaces.

