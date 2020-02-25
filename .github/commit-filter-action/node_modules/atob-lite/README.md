# atob-lite
![](http://img.shields.io/badge/stability-stable-orange.svg?style=flat)
![](http://img.shields.io/npm/v/atob-lite.svg?style=flat)
![](http://img.shields.io/npm/dm/atob-lite.svg?style=flat)
![](http://img.shields.io/npm/l/atob-lite.svg?style=flat)

Smallest/simplest possible means of using atob with both Node and browserify.

In the browser, decoding base64 strings is done using:

``` javascript
var decoded = atob(encoded)
```

However in Node, it's done like so:

``` javascript
var decoded = new Buffer(encoded, 'base64').toString('utf8')
```

You can easily check if `Buffer` exists and switch between the approaches
accordingly, but using `Buffer` anywhere in your browser source will pull
in browserify's `Buffer` shim which is pretty hefty. This package uses
the `main` and `browser` fields in its `package.json` to perform this
check at build time and avoid pulling `Buffer` in unnecessarily.

## Usage

[![NPM](https://nodei.co/npm/atob-lite.png)](https://nodei.co/npm/atob-lite/)

### `decoded = atob(encoded)`

Returns the decoded value of a base64-encoded string.

## License

MIT. See [LICENSE.md](http://github.com/hughsk/atob-lite/blob/master/LICENSE.md) for details.
