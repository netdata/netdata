This is the agent cloud link (ACLK) information file.

### For the `mosquitto` lib:
The installer does not yet have steps to pull and build the local library.
cd project_root

``` text
git clone ssh://git@github.com/netdata/mosquitto mosquitto/
(cd mosquitto/lib && make)      # Ignore the cpp error
```
This will leave mosquitto/lib/libmosquitto.a for the build process to use.