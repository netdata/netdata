# Install netdata

![image10](https://cloud.githubusercontent.com/assets/2662304/14253729/534c6f9c-fa95-11e5-8243-93eb0df719aa.gif)


## Linux Package Managers

- You can install the latest release of netdata, using your package manager in

   - Arch Linux (`sudo pacman -S netdata`)
   - Alpine Linux (`sudo apk add netdata`)
   - Debian Linux (`sudo apt-get install netdata`)
   - Gentoo Linux (`sudo emerge --ask netdata`)
   - OpenSUSE (`sudo zypper install netdata`)
   - Solus Linux (`sudo eopkg install netdata`)
   - Ubuntu Linux >= 18.04 (`sudo apt install netdata`)

  For security and portability reasons, this is the preferred installation method.


## Linux one liner

For all **Linux** systems, you can use this one liner to install the git version of netdata:

```sh
# basic netdata installation
bash <(curl -Ss https://my-netdata.io/kickstart.sh)

# or

# install required packages for all netdata plugins
bash <(curl -Ss https://my-netdata.io/kickstart.sh) all
```

The above is the suggested way of installing the latest netdata and keep it up to date automatically.
The command:

1. detects the distro and **installs the required system packages** for building netdata (will ask for confirmation)
2. downloads the latest netdata source tree to `/usr/src/netdata.git`.
3. installs netdata by running `./netdata-installer.sh` from the source tree.
4. installs `netdata-updater.sh` to `cron.daily`, so your netdata installation will be updated daily (you will get a message from cron only if the update fails).

The `kickstart.sh` script passes all its parameters to `netdata-installer.sh`, so you can add more parameters to change the installation directory, enable/disable plugins, etc (check below).

For automated installs, append a space + `--dont-wait` to the command line. You can also append `--dont-start-it` to prevent the installer from starting netdata. Example:

```sh
bash <(curl -Ss https://my-netdata.io/kickstart.sh) all --dont-wait --dont-start-it
```
