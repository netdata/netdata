declare const macosRelease: {
	/**
	Get the name and version of a macOS release from the Darwin version.

	@param release - By default, the current operating system is used, but you can supply a custom [Darwin kernel version](http://en.wikipedia.org/wiki/Darwin_%28operating_system%29#Release_history), which is the output of [`os.release()`](https://nodejs.org/api/os.html#os_os_release).

	@example
	```
	import * as os from 'os';
	import macosRelease = require('macos-release');

	// On a macOS Sierra system

	macosRelease();
	//=> {name: 'Sierra', version: '10.12'}

	os.release();
	//=> 13.2.0
	// This is the Darwin kernel version

	macosRelease(os.release());
	//=> {name: 'Sierra', version: '10.12'}

	macosRelease('14.0.0');
	//=> {name: 'Yosemite', version: '10.10'}
	```
	*/
	(release?: string): string;

	// TODO: remove this in the next major version, refactor the whole definition to:
	// declare function macosRelease(release?: string): string;
	// export = macosRelease;
	default: typeof macosRelease;
};

export = macosRelease;
