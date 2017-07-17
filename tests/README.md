This readme is a manual on how to get started with unit testing on javascript and nodejs

Original author: BrainDoctor (github), July 2017

# Installation

Tested on Linux Mint 18.2 Sara (Ubuntu/debian derivative)

Make sure you are the user who is developer (permissions, except sudo ofc)

```sh
sudo apt-get install nodejs npm chromium-browser

cd /path/to/your/netdata

npm install
```

That should install the necessary node modules.

Other browsers work too (Chrome, Firefox). However, only the Chromium Browser 59 has been tested for headless unit testing.

## Versions

The commands above leave me with the following versions (July 2017):

 - nodejs: v4.2.6
 - npm: 3.5.2
 - chromium-browser: 59.0.3071.109
 - WebStorm (optional): 2017.1.4

# Configuration

## NPM

The dependencies are installed in `netdata/package.json`. If you install a new NPM module, it gets added here. Future developers just need to execute `npm install` and every dep gets added automatically.

## Karma

Karma configuration is in `tests/web/karma.conf.js`. Documentation is provided via comments.

## WebStorm

If you use the JetBrains WebStorm IDE, you can integrate the karma runtime.

### for Karma (Client side testing)

Headless Chromium:
1. Run > Edit Configurations
2. "+" > Karma
3. - Name: Karma Headless Chromium
   - Configuration file: /path/to/your/netdata/tests/web/karma.conf.js
   - Browsers to start: ChromiumHeadless
   - Node interpreter: /usr/bin/nodejs (MUST be absolute, NVM works too)
   - Karma package: /path/to/your/netdata/node_modules/karma

GUI Chromium is similar:
1. Run > Edit Configurations
2. "+" > Karma
3. - Name: Karma Chromium
   - Configuration file: /path/to/your/netdata/tests/web/karma.conf.js
   - Browsers to start: Chromium
   - Node interpreter: /usr/bin/nodejs (MUST be absolute, NVM works too)
   - Karma package: /path/to/your/netdata/node_modules/karma

You may add other browsers too (comma separated). With the "Browsers to start" field you can override any settings in karma.conf.js.

Also it is recommended to install WebStorm IDE Extension/Addon to Chrome/Chromium for awesome debugging.

### for node.d plugins (nodejs)

1. Run > Edit Configurations
2. "+" > Node.js
3. - Name: Node.d plugins
   - Node interpreter: /usr/bin/nodejs (MUST be absolute, NVM works too)
   - JavaScript file: node_modules/jasmine-node/bin/jasmine-node
   - Application parameters: --captureExceptions tests/node.d

**ATTENTION**

The jasmine-node npm package includes an outdated jasmine on version 1.3.1.

I haven't figured out yet how to switch to newer 2.6 versions. So it may contain bugs or lacks features.

# Running

## In WebStorm

Just run the configured run configurations and they produce nice test trees:


## From CLI

### Karma

```sh
cd /path/to/your/netdata

nodejs ./node_modules/karma/bin/karma start tests/web/karma.conf.js --single-run=true --browsers=ChromiumHeadless
```
will start the karma server, start chromium in headless mode and exit.

If a test fails, it produces even a stack trace:


### Node.d plugins

```sh
cd /path/to/your/netdata

nodejs node_modules/jasmine-node/bin/jasmine-node --captureExceptions tests/node.d
```

will run the tests in `tests/node.d` and produce a stacktrace too on error:


## Coverage

### Karma

A nice HTML is produced from Karma which shows which code paths were executed. It is located somewhere in `/path/to/your/netdata/coverage/`

### Node.d

Apparently, jasmine-node can produce a junit report with the `--junitreport` flag. But that output was not very useful. Maybe it's configurable?

## CI

The karma and node.d runners can be integrated in Travis (AFAIK), but that is outside my ability.

Note: Karma is for browser-testing. On a build server, no GUI or browser might by available, unless browsers support headless mode.