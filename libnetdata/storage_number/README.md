# netdata storage number

Although `netdata` does all its calculations using `long double`, it stores all values using
a **custom-made 32-bit number**.

This custom-made number can store in 29 bits values from `-167772150000000.0` to  `167772150000000.0`
with a precision of 0.00001 (yes, it's a floating point number, meaning that higher integer values
have less decimal precision) and 3 bits for flags.

This provides an extremely optimized memory footprint with just 0.0001% max accuracy loss.

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Flibnetdata%2Fstorage_number%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)]()
