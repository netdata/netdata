/**
Get the name of a Windows version from the release number: `5.1.2600` → `XP`.

@param release - By default, the current OS is used, but you can supply a custom release number, which is the output of [`os.release()`](https://nodejs.org/api/os.html#os_os_release).

Note: Most Windows Server versions cannot be detected based on the release number alone. There is runtime detection in place to work around this, but it will only be used if no argument is supplied, or the supplied argument matches `os.release()`.

@example
```
import * as os from 'os';
import windowsRelease = require('windows-release');

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
*/
declare function windowsRelease(release?: string): string;

export = windowsRelease;
