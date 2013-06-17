netdata
=======

linux network traffic web monitoring


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

