# windows-release [![Build Status](https://travis-ci.org/sindresorhus/windows-release.svg?branch=master)](https://travis-ci.org/sindresorhus/windows-release)

> Get the name of a Windows version from the release number: `5.1.2600` → `XP`


## Install

```
$ npm install windows-release
```


## Usage

```js
const os = require('os');
const windowsRelease = require('windows-release');

// On a Windows XP system

windowsRelease();
//=> 'XP'

os.release();
//=> '5.1.2600'

windowsRelease(os.release());
//=> 'XP'

windowsRelease('4.9.3000');
//=> 'ME'
```


## API

### windowsRelease([release])

#### release

Type: `string`

By default, the current OS is used, but you can supply a custom release number, which is the output of [`os.release()`](https://nodejs.org/api/os.html#os_os_release).

Note: Most Windows Server versions cannot be detected based on the release number alone. There is runtime detection in place to work around this, but it will only be used if no argument is supplied, or the supplied argument matches `os.release()`.


## Related

- [os-name](https://github.com/sindresorhus/os-name) - Get the name of the current operating system
- [macos-release](https://github.com/sindresorhus/macos-release) - Get the name and version of a macOS release from the Darwin version


## License

MIT © [Sindre Sorhus](https://sindresorhus.com)
