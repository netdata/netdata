# Redistributed software

Netdata copyright info:<br/>
Copyright 2018-2025 Netdata Inc.<br/>
Released under [GPL v3 or later](https://raw.githubusercontent.com/netdata/netdata/master/LICENSE).

Netdata uses SPDX license tags to identify the license for its files.
Individual licenses referenced in the tags are available on the [SPDX project site](http://spdx.org/licenses/).

Netdata redistributes the Netdata Cloud UI, licensed under [Netdata Cloud UI License v1.0 (NCUL1)](https://app.netdata.cloud/LICENSE.txt). Netdata Cloud UI includes [third party open-source software](https://app.netdata.cloud/3D_PARTY_LICENSES.txt).

Netdata redistributes the following third-party software.
We have decided to redistribute all these, instead of using them through a CDN, to allow Netdata to work in cases where Internet connectivity is not available.

**Netdata Agent**:

| Name                                                                    | Copyright                                                                          | License                                                                                  |
|-------------------------------------------------------------------------|------------------------------------------------------------------------------------|------------------------------------------------------------------------------------------|
| [Kolmogorov-Smirnov distribution](http://simul.iro.umontreal.ca/ksdir/) | Copyright March 2010 by Université de Montréal, Richard Simard and Pierre L'Ecuyer | [GPL 3.0](https://www.gnu.org/licenses/gpl-3.0.en.html)                                  |
| [xxHash](https://github.com/Cyan4973/xxHash)                            | Copyright (c) 2012-2021 Yann Collet                                                | [BSD](https://github.com/Cyan4973/xxHash/blob/dev/LICENSE)                               |
| [Judy Arrays](https://sourceforge.net/projects/judy/)                   | Copyright (C) 2000 - 2002 Hewlett-Packard Company                                  | [LGPLv2](https://www.gnu.org/licenses/old-licenses/lgpl-2.1.en.html)                     |
| [dlib](https://github.com/davisking/dlib/)                              | Copyright (C) 2019  Davis E. King (davis@dlib.net), Nils Labugt                    | [Boost Software License](https://github.com/davisking/dlib/blob/master/dlib/LICENSE.txt) |

---

**python.d.plugin**:

| Name                                                                                                                                                                                     | Copyright                                             | License                                                          |
|------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|-------------------------------------------------------|------------------------------------------------------------------|
| [PyYAML](https://pypi.org/project/PyYAML/)                                                                                                                                               | Copyright 2006, Kirill Simonov                        | [MIT](https://github.com/yaml/pyyaml/blob/master/LICENSE)        |
| [urllib3](https://github.com/shazow/urllib3)                                                                                                                                             | Copyright 2008-2016 Andrey Petrov and contributors    | [MIT](https://github.com/shazow/urllib3/blob/master/LICENSE.txt) |
| [Utilities for writing code that runs on Python 2 and 3](https://raw.githubusercontent.com/netdata/netdata/master/src/collectors/python.d.plugin/python_modules/urllib3/packages/six.py) | Copyright (c) 2010-2015 Benjamin Peterson             | [MIT](https://github.com/benjaminp/six/blob/master/LICENSE)      |
| [monotonic](https://github.com/atdt/monotonic)                                                                                                                                           | Copyright 2014, 2015, 2016 Ori Livneh                 | [Apache-2.0](http://www.apache.org/licenses/LICENSE-2.0)         |
| [filelock](https://github.com/benediktschmitt/py-filelock)                                                                                                                               | Copyright 2015, Benedikt Schmitt                      | [Unlicense](https://unlicense.org/)                              |

---

**go.d.plugin**:

| Name                                               | Copyright                   | License                                                             |
|----------------------------------------------------|-----------------------------|---------------------------------------------------------------------|
| [lmsensors](https://github.com/mdlayher/lmsensors) | Copyright 2016, Matt Layher | [MIT](https://github.com/mdlayher/lmsensors/blob/master/LICENSE.md) |

---

**Old User Interface (Dashboard v1)**:

| Name                                                                            | Copyright                                                   | License                                                                                                                                                                                                           |
|---------------------------------------------------------------------------------|-------------------------------------------------------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| [Dygraphs](http://dygraphs.com/)                                                | Copyright 2009, Dan Vanderkam                               | [MIT](http://dygraphs.com/legal.html)                                                                                                                                                                             |
| [Easy Pie Chart](https://rendro.github.io/easy-pie-chart/)                      | Copyright 2013, Robert Fleischmann                          | [MIT](https://github.com/rendro/easy-pie-chart/blob/master/LICENSE)                                                                                                                                               |
| [Gauge.js](http://bernii.github.io/gauge.js/)                                   | Copyright, Bernard Kobos                                    | [MIT](https://github.com/getgauge/gauge-js/blob/master/LICENSE)                                                                                                                                                   |
| [d3pie](https://github.com/benkeen/d3pie)                                       | Copyright (c) 2014-2015 Benjamin Keen                       | [MIT](https://github.com/benkeen/d3pie/blob/master/LICENSE)                                                                                                                                                       |
| [jQuery Sparklines](http://omnipotent.net/jquery.sparkline/)                    | Copyright 2009-2012, Splunk Inc.                            | [New BSD](http://opensource.org/licenses/BSD-3-Clause)                                                                                                                                                            |
| [Peity](http://benpickles.github.io/peity/)                                     | Copyright 2009-2015, Ben Pickles                            | [MIT](https://github.com/benpickles/peity/blob/master/LICENCE)                                                                                                                                                    |
| [morris.js](http://morrisjs.github.io/morris.js/)                               | Copyright 2013, Olly Smith                                  | [Simplified BSD](http://morrisjs.github.io/morris.js/)                                                                                                                                                            |
| [Raphaël](http://dmitrybaranovskiy.github.io/raphael/)                          | Copyright 2008, Dmitry Baranovskiy                          | [MIT](http://dmitrybaranovskiy.github.io/raphael/license.html)                                                                                                                                                    |
| [C3](http://c3js.org/)                                                          | Copyright 2013, Masayuki Tanaka                             | [MIT](https://github.com/masayuki0812/c3/blob/master/LICENSE)                                                                                                                                                     |
| [D3](http://d3js.org/)                                                          | Copyright 2015, Mike Bostock                                | [BSD](http://opensource.org/licenses/BSD-3-Clause)                                                                                                                                                                |
| [jQuery](https://jquery.org/)                                                   | Copyright 2015, jQuery Foundation                           | [MIT](https://jquery.org/license/)                                                                                                                                                                                |
| [Bootstrap](http://getbootstrap.com/getting-started/)                           | Copyright 2015, Twitter                                     | [MIT](https://github.com/twbs/bootstrap/blob/v4-dev/LICENSE)                                                                                                                                                      |
| [Bootstrap Toggle](http://www.bootstraptoggle.com/)                             | Copyright (c) 2011-2014 Min Hur, The New York Times Company | [MIT](https://github.com/minhur/bootstrap-toggle/blob/master/LICENSE)                                                                                                                                             |
| [Bootstrap-slider](http://seiyria.com/bootstrap-slider/)                        | Copyright 2017 Kyle Kemp, Rohit Kalkur, and contributors    | [MIT](https://github.com/seiyria/bootstrap-slider/blob/master/LICENSE.md)                                                                                                                                         |
| [bootstrap-table](http://bootstrap-table.wenzhixin.net.cn/)                     | Copyright (c) 2012-2016 Zhixin Wen                          | [MIT](https://github.com/wenzhixin/bootstrap-table/blob/master/LICENSE)                                                                                                                                           |
| [tableExport.jquery.plugin](https://github.com/hhurz/tableExport.jquery.plugin) | Copyright (c) 2015,2016 hhurz                               | [MIT](https://github.com/hhurz/tableExport.jquery.plugin/blob/master/LICENSE)                                                                                                                                     |
| [perfect-scrollbar](https://jamesflorentino.github.io/nanoScrollerJS/)          | Copyright 2016, Hyunje Alex Jun and other contributors      | [MIT](https://github.com/noraesae/perfect-scrollbar/blob/master/LICENSE)                                                                                                                                          |
| [FontAwesome](https://github.com/FortAwesome/Font-Awesome)                      | Created by Dave Gandy                                       | Font: [SIL OFL 1.1](http://scripts.sil.org/OFL), Icon: [Creative Commons Attribution 4.0 (CC-BY 4.0)](https://creativecommons.org/licenses/by/4.0/), Code: [MIT](http://opensource.org/licenses/mit-license.html) |
| [node-extend](https://github.com/justmoon/node-extend)                          | Copyright 2014, Stefan Thomas                               | [MIT](https://github.com/justmoon/node-extend/blob/master/LICENSE)                                                                                                                                                |
| [pixl-xml](https://github.com/jhuckaby/pixl-xml)                                | Copyright 2015, Joseph Huckaby                              | [MIT](https://github.com/jhuckaby/pixl-xml#license)                                                                                                                                                               |
| [pako](http://nodeca.github.io/pako/)                                           | Copyright 2014-2017 Vitaly Puzrin and Andrei Tuputcyn       | [MIT](https://github.com/nodeca/pako/blob/master/LICENSE)                                                                                                                                                         |
| [lz-string](http://pieroxy.net/blog/pages/lz-string/index.html)                 | Copyright 2013 Pieroxy                                      | [WTFPL](http://pieroxy.net/blog/pages/lz-string/index.html#inline_menu_10)                                                                                                                                        |
| [clipboard-polyfill](https://github.com/lgarron/clipboard-polyfill)             | Copyright (c) 2014 Lucas Garron                             | [MIT](https://github.com/lgarron/clipboard-polyfill/blob/master/LICENSE.md)                                                                                                                                       |
