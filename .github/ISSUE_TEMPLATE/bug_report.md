---
name: Bug report
about: Create a bug report to help us improve
labels: bug, needs triage
---

<!--
When creating a bug report please:
- Verify first that your issue is not already reported on GitHub.
- Test if the latest release and master branch are affected too.
-->

##### Bug report summary
<!-- Provide a clear and concise description of the bug you're experiencing. -->

##### OS / Environment
<!--
Provide as much information about your environment (which operating system and distribution you're using, if Netdata is running in a container, etc.)
as possible to allow us reproduce this bug faster.

To get this information, execute the following commands based on your operating system:
- uname -a; grep -Hv "^#" /etc/*release  # Linux
- uname -a; uname -K                     # BSD
- uname -a; sw_vers                      # macOS

Place the output from the command in the code section below.  
 -->
```

```

##### Netdata version
<!--
Provide output of `netdata -V`.
 
If Netdata is running, execute: $(ps aux | grep -E -o "[a-zA-Z/]+netdata ") -V
 -->
 

##### Component Name
<!--
Let us know which component is affected by the bug. Our code is structured according to its component,
so the component name is the same as the top level directory of the repository.
For example, a bug in the dashboard would be under the web component.
-->

##### Steps To Reproduce
<!--
Describe how you found this bug and how we can reproduce it, preferably with a minimal test-case scenario.
If you'd like to attach larger files, use gist.github.com and paste in links.
-->

1. ...
2. ...

##### Expected behavior
<!-- Provide a clear and concise description of what you expected to happen. -->
