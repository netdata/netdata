# AM2320
This module will display a graph of the temperture and humity from a AM2320 sensor

**Requirements:**
 - Adafruit Circuit Python AM2320 library
 - Adafruit AM2320 I2C sensor
 - Python 3
 - Raspberry Pi board

It produces the following charts:
1. **Temperature** 
2. **Humidity**

## configuration

Hardware install:
Connect the am2320 to the Raspbery Pi I2C pins

Raspberry Pi 3B/4 Pins:

Board 3.3V (pin 1) to sensor VIN (pin 1)
Board SDA (pin 3) to sensor SDA (pin 2)
Board GND (pin 6) to sensor GND (pin 3)
Board SCL (pin 5) to sensor SCL (pin 4)

You may also need to add two I2C pullup resistors if your board does not already have them. The Raspberry Pi does have internal pullup resistors but it doesnt hurt to add them anyway. You can use 2.2K - 10K but we will just use 10K. The resistors go from VDD to SCL and SDA each.

Software install:
sudo pip3 install adafruit-circuitpython-am2320
sudo usermod -G I2C netdata
edit /etc/netdata/netdata.conf
find [plugin:python.d]
add  command options = -ppython3
Save file
move am2320.chart.py to /usr/libexec/netdata/python.d 
restart netdata service
check the dashboard
