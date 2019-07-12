# Netdata Style Guide

This in-progress style guide establishes editorial guidelines for anyone who wants to write documentation for Netdata products.

## Table of Contents

- [Welcome!](#welcome)
- [Goals of the style guide](#goals-of-the-Netdata-style-guide)
  - [Breaking the rules](#breaking-the-rules)
- [Style guide quickstart](#style-guide-quickstart)


## Welcome!

Here at Netdata, we believe proper documentation is essential to the success of any open-source project, and we're commited to do just that. We're relying on our community to help us create new documentation, improve existing pages, and put everything we do to the test in real-world scenarios.

We're here to make developers, sysadmins, and DevOps engineers better at their jobs, after all.

If you'd like to contribute, start by reading the [guide on contributing to documentation](contributing.md) and then read the rest of this style guide.


## Goals of the Netdata style guide

An editorial style guide tries to establish a few important standards:

- Consistency
- High-quality writing
- Conciseness
- Accessibility

By following these principles, we can make 


### Breaking the rules

None of the rules described in this style guide are absolute.

You'll come across broken rules across the existing documentation, and improvements will often require these types of deviates as well. **We welcome rule-breaking if it creates better, more accessible documentation.**

However, be aware that if you deviate too far from these established rules you may be asked to justify your decisions during the PR review process.


## Style guide quickstart

This style guide is pretty overwhelming. The following list represents a few of the more essential points and rules featured in this style guide.

- 


## General principles



###Mentioning future features/releases

Documentation is meant to describe the product as-is, not as it will be or could be in the future. Thus Netdata documentation generally avoids talking about future features or products, even if we know they are certain.

An exception can be made for documenting beta features that don't yet fully work as expected. By briefly describing 


## Grammar 

### Tense



## Markdown syntax

### References to clicking links or buttons

If you need to instruct the user to click a button or a link inside a Netdata interface, you should do two things:

1. Be as descriptive as possible about where to find the link/button, and if relevant, what the link/button looks like.
2. Directly reference the text of the link/button inside of an inline code block.

Use Markdown's inline code tag to turn the text of the link/button into an inline code block:

``` markdown
Click on the `Sign in` button in the bottom-right corner of the popup window.
```


### Language-specific syntax highlighting in code blocks

Currently our documentation uses the 

````
```c
#include "../daemon/common.h"
#include "registry_internals.h"

#define REGISTRY_STATUS_OK "ok"
#define REGISTRY_STATUS_FAILED "failed"
#define REGISTRY_STATUS_DISABLED "disabled"
```
````


### Adding line numbers to code blocks

````
```c  linenums="1"
#include "../daemon/common.h"
#include "registry_internals.h"

#define REGISTRY_STATUS_OK "ok"
#define REGISTRY_STATUS_FAILED "failed"
#define REGISTRY_STATUS_DISABLED "disabled"
```
````

```c  linenums="1"
#include "../daemon/common.h"
#include "registry_internals.h"

#define REGISTRY_STATUS_OK "ok"
#define REGISTRY_STATUS_FAILED "failed"
#define REGISTRY_STATUS_DISABLED "disabled"
```

### Highlighting specific lines in code blocks

````
```py3 hl_lines="1 3"
#include "../daemon/common.h"
#include "registry_internals.h"

#define REGISTRY_STATUS_OK "ok"
#define REGISTRY_STATUS_FAILED "failed"
#define REGISTRY_STATUS_DISABLED "disabled"
```
````

```c hl_lines="1 3"
#include "../daemon/common.h"
#include "registry_internals.h"

#define REGISTRY_STATUS_OK "ok"
#define REGISTRY_STATUS_FAILED "failed"
#define REGISTRY_STATUS_DISABLED "disabled"
```

````
```py3 hl_lines="1-2 4"
#include "../daemon/common.h"
#include "registry_internals.h"

#define REGISTRY_STATUS_OK "ok"
#define REGISTRY_STATUS_FAILED "failed"
#define REGISTRY_STATUS_DISABLED "disabled"
```
````

```c hl_lines="1-2 4"
#include "../daemon/common.h"
#include "registry_internals.h"

#define REGISTRY_STATUS_OK "ok"
#define REGISTRY_STATUS_FAILED "failed"
#define REGISTRY_STATUS_DISABLED "disabled"
```