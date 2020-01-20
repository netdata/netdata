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
<!-- Provide a clear and concise description of what the bug. -->

##### OS / Environment
<!--
Provide as much information about your environment (OS distribution, running in container, etc.)
as possible to allow us reproduce this bug faster.

To get this information execute:
- uname -a; grep -Hv "^#" /etc/*release  # linux/bsd
- uname -a; sw_vers                      # macOS

Place output in the code section.  
 -->
```

```

##### Netdata version
<!--
Provide output of netdata -V.
 
If netdata is running execute: $(ps aux | grep -E -o "[a-zA-Z/]+netdata ") -V
 -->
 

##### Component Name
<!--
Write which component is affected. We group our components the same way our code is structured so basically: 
component name = dir in top level directory of repository.
-->

##### Steps To Reproduce
<!--
Describe how you found this bug and how we can reproduce it. Preferable with a minimal test-case scenario.
You can paste gist.github.com links for larger files.
-->

1. ...
2. ...

##### Expected behavior
<!-- Provide a clear and concise description of what you expected to happen. -->