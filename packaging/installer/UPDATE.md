# Updating netdata after its installation

![image8](https://cloud.githubusercontent.com/assets/2662304/14253735/536f4580-fa95-11e5-9f7b-99112b31a5d7.gif)


We suggest to keep your netdata updated. We are actively developing it and you should always update to the latest version.

The update procedure depends on how you installed it:

## You downloaded it from github using git

### Manual update

The installer `netdata-installer.sh` generates a `netdata-updater.sh` script in the directory you downloaded netdata.
You can use this script to update your netdata installation with the same options you used to install it in the first place.
Just run it and it will download and install the latest version of netdata. The same script can be put in a cronjob to update your netdata at regular intervals.

```sh
# go to the git downloaded directory
cd /path/to/git/downloaded/netdata

# run the updater
./netdata-updater.sh
```

_Netdata will be restarted with the new version._

If you don't have this script (e.g. you deleted the directory where you downloaded netdata), just follow the **[[Installation]]** instructions again. The installer preserves your configuration. You can also update netdata to the latest version by hand, using this:

```sh
# go to the git downloaded directory
cd /path/to/git/downloaded/netdata

# download the latest version
git pull

# rebuild it, install it, run it
./netdata-installer.sh
```

_Netdata will be restarted with the new version._

Keep in mind, netdata may now have new features, or certain old features may now behave differently. So pay some attention to it after updating.

### Auto-update

_Please, consider the risks of running an auto-update. Something can always go wrong. Keep an eye on your installation, and run a manual update if something ever fails._

You can call `netdata-updater.sh` from a cron-job. A successful update will not trigger an email from cron.

```sh
# Edit your cron-jobs
crontab -e

# add a cron-job at the bottom. This one will update netdata every day at 6:00AM:
# update netdata
0 6 * * * /path/to/git/downloaded/netdata/netdata-updater.sh
```

## You downloaded a binary package

If you installed it from a binary package, the best way is to **obtain a newer copy** from the source you got it in the first place.

If a newer version of netdata is not available from the source you got it, we suggest to uninstall the version you have and follow the **[[Installation]]** instructions for installing a fresh version of netdata.








[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Finstaller%2FUPDATE&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)]()
