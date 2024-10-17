# Netdata storage number

Although `netdata` does all its calculations using `long double`, it stores all values using
a **custom-made 32-bit number**.

This custom-made number can store in 29 bits values from `-167772150000000.0` to  `167772150000000.0`
with a precision of 0.00001 (yes, it's a floating point number, meaning that higher integer values
have less decimal precision) and 3 bits for flags.

This provides an extremely optimized memory footprint with just 0.0001% max accuracy loss.


