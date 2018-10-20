# alerta.io notifications

The alerta monitoring system is a tool used to consolidate and de-duplicate alerts from multiple sources for quick ‘at-a-glance’ visualisation. With just one system you can monitor alerts from many other monitoring tools on a single screen.

![](http://docs.alerta.io/en/latest/_images/alerta-screen-shot-3.png)

When receiving alerts from multiple sources you can quickly become overwhelmed. With Alerta any alert with the same environment and resource is considered a duplicate if it has the same severity. If it has a different severity it is correlated so that you only see the most recent one. Awesome.

main site http://www.alerta.io

We can send Netadata alarms to Alerta so yo can see in one place alerts coming from many Netdata hosts or also from a multihost Netadata configuration.\
The big advantage over other notifications method is that you have in a main view all active alarms with only las state, but you can also search history.

## Setting up an Alerta server with Ubuntu 16.04

Here we will set a basic Alerta server to test it with Netdata alerts.\
More advanced configurations are out os scope of this tutorial.

source: http://alerta.readthedocs.io/en/latest/gettingstarted/tutorial-1-deploy-alerta.html

I recommend to set up the server in a separated server, VM or container.\
If you have other Nginx or Apache server in your organization, I recommend to proxy to this new server.

Set us as root for easiest working
```
sudo su
cd
```

Install Mongodb https://docs.mongodb.com/manual/tutorial/install-mongodb-on-ubuntu/
```
apt-key adv --keyserver hkp://keyserver.ubuntu.com:80 --recv 2930ADAE8CAF5059EE73BB4B58712A2291FA4AD5
echo "deb [ arch=amd64,arm64 ] https://repo.mongodb.org/apt/ubuntu xenial/mongodb-org/3.6 multiverse" | tee /etc/apt/sources.list.d/mongodb-org-3.6.list
apt-get update
apt-get install -y mongodb-org
systemctl enable mongod
systemctl start mongod
systemctl status mongod
```

Install Nginx and Alerta uwsgi
```
apt-get install -y python-pip python-dev nginx
pip install alerta-server uwsgi
```

Install web console
```
cd /var/www/html
mkdir alerta
cd alerta
wget -q -O - https://github.com/alerta/angular-alerta-webui/tarball/master | tar zxf -
mv alerta*/app/* .
cd
```
## Services configuration

Create a wsgi python file
```
nano /var/www/wsgi.py
```
fill with
```
from alerta import app
```
Create uWsgi configuration file
```
nano /etc/uwsgi.ini
```
fill with
```
[uwsgi]
chdir = /var/www
mount = /alerta/api=wsgi.py
callable = app
manage-script-name = true

master = true
processes = 5
logger = syslog:alertad

socket = /tmp/uwsgi.sock
chmod-socket = 664
uid = www-data
gid = www-data
vacuum = true

die-on-term = true
```
Create a systemd configuration file
```
nano /etc/systemd/system/uwsgi.service
```
fill with
```
[Unit]
Description=uWSGI service

[Service]
ExecStart=/usr/local/bin/uwsgi --ini /etc/uwsgi.ini

[Install]
WantedBy=multi-user.target
```
enable service
```
systemctl start uwsgi
systemctl status uwsgi
systemctl enable uwsgi
```
Configure nginx to serve Alerta as a uWsgi application on /alerta/api
```
nano /etc/nginx/sites-enabled/default
```
fill with
```
server {
        listen 80 default_server;
        listen [::]:80 default_server;

        location /alerta/api { try_files $uri @alerta/api; }
        location @alerta/api {
            include uwsgi_params;
            uwsgi_pass unix:/tmp/uwsgi.sock;
            proxy_set_header Host $host:$server_port;
            proxy_set_header X-Real-IP $remote_addr;
            proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        }

        location / {
            root /var/www/html;
        }
}
```
restart nginx
```
service nginx restart
```
## Config web console
```
nano /var/www/html/config.js
```
fill with
```
'use strict';

angular.module('config', [])
  .constant('config', {
    'endpoint'    : "/alerta/api",
    'provider'    : "basic",
    'colors'      : {},
    'severity'    : {},
    'audio'       : {}
  });
```

## Config Alerta server

source: http://alerta.readthedocs.io/en/latest/configuration.html

Create a random string to use as SECRET_KEY
```
cat /dev/urandom | tr -dc A-Za-z0-9_\!\@\#\$\%\^\&\*\(\)-+= | head -c 32 && echo
```
will output something like
```
0pv8Bw7VKfW6avDAz_TqzYPme_fYV%7g
```
Edit alertad.conf
```
nano /etc/alertad.conf
```
fill with (take care about all single quotes)
```
BASE_URL='/alerta/api'
AUTH_REQUIRED=True
SECRET_KEY='0pv8Bw7VKfW6avDAz_TqzYPme_fYV%7g'
ADMIN_USERS=['<here put you email for future login>']
```

restart
```
systemctl restart uwsgi
```

* go to console to http://yourserver/alerta/
* go to Login -> Create an account
* use your email for login so and administrative account will be created

## create an API KEY

You need an API KEY to send messages from any source.\
To create an API KEY go to Configuration -> Api Keys\
Then create a API KEY with write permisions.

## configure Netdata to send alarms to Alerta

On your system run:

```
/etc/netdata/edit-config health_alarm_notify.conf
```

and set

```
# enable/disable sending alerta notifications
SEND_ALERTA="YES"

# here set your alerta server API url
# this is the API url you defined when installed Alerta server, 
# it is the same for all users. Do not include last slash.
ALERTA_WEBHOOK_URL="http://yourserver/alerta/api"

# Login with an administrative user to you Alerta server and create an API KEY
# with write permissions.
ALERTA_API_KEY="you last created API KEY"

# you can define environments in /etc/alertad.conf option ALLOWED_ENVIRONMENTS
# standard environments are Production and Development
# if a role's recipients are not configured, a notification will be send to
# this Environment (empty = do not send a notification for unconfigured roles):
DEFAULT_RECIPIENT_ALERTA="Production"
```

## Test alarms

We can test alarms with standard
```
sudo su -s /bin/bash netdata
/opt/netdata/netdata-plugins/plugins.d/alarm-notify.sh test
exit
```
But the problem is that Netdata will send 3 alarms, and because last alarm is "CLEAR" you will not se them in main Alerta page, you need to select to see "closed" alarma in top-right lookup.

A little change in alarm-notify.sh that let us test each state one by one will be useful.