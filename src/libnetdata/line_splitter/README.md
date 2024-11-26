# Line splitter

Common code that handles line splitting into words.

The pluginsd line splitter is generated with re2c because it leads to 3x the
performance of the hand-written line splitter:

`re2c re2c_line_splitter.re.c -o re2c_line_splitter.c`
