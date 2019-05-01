# Updating netdata after its installation

![image8](https://cloud.githubusercontent.com/assets/2662304/14253735/536f4580-fa95-11e5-9f7b-99112b31a5d7.gif)


We suggest to keep your netdata updated. We are actively developing it and you should always update to the latest version.

The update procedure depends on how you installed it:

## You downloaded it from github using git

### Manual update to get the latest git commit

netdata versions older than `v1.12.0-rc2-52` had a `netdata-updater.sh` script in the root directory of the source code, which has now been deprecated. The manual process that works for all versions to get the latest commit in git is to use the `netdata-installer.sh`. The installer preserves your custom configuration and updates the information of the installation in the `.environment` file under the user configuration directory.

```sh
# go to the git downloaded directory
cd /path/to/git/downloaded/netdata

# update your local copy
git pull

# run the netdata installer
sudo ./netdata-installer.sh
```

_Netdata will be restarted with the new version._

Keep in mind, netdata may now have new features, or certain old features may now behave differently. So pay some attention to it after updating.

### Manual update to get the latest nightly build

The `kickstart.sh` one-liner will do a one-time update to the latest nightly build, if executed as follows:
```
bash <(curl -Ss https://my-netdata.io/kickstart.sh) --no-updates
```

### Auto-update

_Please, consider the risks of running an auto-update. Something can always go wrong. Keep an eye on your installation, and run a manual update if something ever fails._

Calling the `netdata-installer.sh` with the `--auto-update` or `-u` option will create the `netdata-updater` script under 
either  `/etc/cron.daily/`, or `/etc/periodic/daily/`. Whenever the `netdata-updater` is executed, it checks if a newer nightly build is available and then handles the download, installation and netdata restart.  

Note that after Jan 2019, the `kickstart.sh` one-liner `bash <(curl -Ss https://my-netdata.io/kickstart.sh)` calls the `netdata-installer.sh` with the auto-update option. So if you just run the one-liner without options once, your netdata will be kept auto-updated.


## You downloaded a binary package

If you installed it from a binary package, the best way is to **obtain a newer copy** from the source you got it in the first place. This includes the static binary installation via `kickstart-base64.sh`, which would need to be executed again.

If a newer version of netdata is not available from the source you got it, we suggest to uninstall the version you have and follow the [installation](README.md) instructions for installing a fresh version of netdata.


[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Finstaller%2FUPDATE&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)]()
