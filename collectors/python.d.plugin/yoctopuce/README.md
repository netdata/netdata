# yoctopuce

Fetches data from Yoctopuce sensor modules.

Provides one chart each for the following sensor interface types:

1. Ambient Light Sensors
2. Temperature Sensors
3. Humidity Sensors
4. Pressure Sensors
4. Current Sensors
5. Voltage Sensors
6. VOC (volatile organic compounds) Sensors
7. Carbon Dioxide Sensors


### configuration

The default configuration attempts to connect to a yoctopuce
Virtual Hub instance running on the local loopback interface, and
automatically enumerates all the supported sensors connected to the hub.

The `hub` value specifies how to connect to the sensors, using one of
the following formats:

* `usb`: This says to just talk directly to locally connected modules
directly over USB.  This is the fastest option, but it also runs the risk
of deadlocking the modules (or even the whole system if you're really
unlucky) if you try to access them simultaneously from another program.
Because this is risky, it is not the default behavior.
* A hostname or IP address: This says to connect to a yoctopuce network
hub (either a running instance of the yoctopuce Virtual Hub software,
or one of their networked hardware hubs).
* A HTTP URL: This is needed if the hub you are trying to
connect to has access control enabled.  The form should be
`http://username:password@address:port`, with the port being optional.

The `scan` configuration item controls whether or not to automatically
enumerate all supported sensors connected to the hub.  Sensors detected
this way will have corresponding dimensions created based on what the
Yoctopuce API considers the 'friendly name' of the sensor.  This will
use the sensor's logical names if you have them set, or hardware-level
identification if you don't.

If `scan` is false, then you need to specify a list of devices of each
type to try to connect to with the `search` item.  Each item in this list
should have two values, `id` and `name`.  `id` is used to look for the
device (and should be the same as the name of the dimension created for
the device when using scan mode), while `name` specifies what to name
the dimension.

Here's what the default config looks like:

```yaml
hub: 127.0.0.1
scan: yes
```

And here's an example that talks two temperature sensors and one light
sensor connected to the local system directly via USB:

```yaml
hub: usb
scan: no
search:
  light:
    - id: 'LIGHT03-123456.light1'
      name: 'light'
  temperature:
    - id: 'TEMP02-123456.temp1'
      name: 'inside'
    - id: 'TEMP02-123456.temp2'
      name: 'outside'
```


### notes

* You will need the Yoctopuce Python library.  If your distribution
  doesn't provide this, you can get an up-to-date copy from pypi.org.
* Seriously, don't use the USB mode unless you're 100% certain that
  you will not be trying to access the modules from _ANY_ other software.
  It really can cause deadlocks (among other things).
* Calling out to USB devices is expensive.  Because of this, this module
  runs only once every 5 seconds by default.  Only lower this if you
  really need it to collect data more frequently and are willing to deal
  with the performance implications.
