# Alert Guides

> This directory contains outdated guides on our alerts, and should be revisited and updated.

# Review process

Each guide must pass one technical review and one phrasal/grammatical review. Technical reviews will be equally assigned to the senior staff - considering the team's priorities.

The review should focus on:

- Is the guide useful and on topic?
- Is it accurate? Does it have any unclear technical instruction? Please note which part.
- Is it easy to read? If not, please note which part was not.
- Is it rich in information? If not, please note which part we should explain more.

Any other suggestions are welcomed.

# Threshold presentation

- Some alerts might use the conditional operator to determine on which state the alarm is. Let's break down this block
  of code:

```sh
warn: $this > (($status >= $WARNING)  ? (75) : (85))
crit: $this > (($status == $CRITICAL) ? (85) : (95))
```

In the above:

If the alarm is currently a warning, then the threshold for being considered a warning is 75, otherwise it's 85.

If the alarm is currently critical, then the threshold for being considered critical is 85, otherwise it's 95.

Which in turn, results in the following behavior:

While the value is rising, it will trigger a warning when it exceeds 85, and a critical alert when it exceeds 95.

While the value is falling, it will return to a warning state when it goes below 85, and a normal state when it goes
below 75.

If the value is fluctuating between 80 and 90, then it will trigger a warning the first time it goes above 85
and will remain a warning until it goes below 75 (or goes above 85).

If the value is fluctuating between 90 and 100, then it will trigger a critical alert first time it goes
above 95 and will remain a critical alert until it goes below 85 - at which point it will return to being a warning.

- In our guides we should write a sentence similar to the example below:

> - This alert is raised in a warning state when the percentage of used IPv4 TCP connections is greater than 80% and
    > less than 90%.
>- If the metric exceeds 90%, then the alert gets raised in critical state.

Note: the thresholds can be customized, the above is just an example using the conditional operator.

<br>

**We will construct each post following the template below as closely as possible:**

<details>
<summary>Template</summary>

# alarm_name

## OS: <OS_Name>

The initial topic for each alarm should provide as much of the following information as possible:

- How this component works.

- A description of what is being monitored.

- A general description of what the specific alarm is about.

- Negative effects of this abnormality.

<details>
<summary>More information about a highly technical detail</summary>
We organize highly technical details into collapsible contents to keep the reader focused.
</details>

<details>
<summary>References and sources</summary>

1. [Descriptive sentece of the link1](https://community.netdata.cloud/)
2. [Descriptive sentece of the link2](https://community.netdata.cloud/)

General guidelines on the referenced resources

1. Starting the guide with a quote, we use italic.

_Lorem ipsum dolor sit amet, consectetur adipisicing elit, sed do eiusmod tempor incididunt ut
labore et dolore magna aliqua._ <sup>[1](https://community.netdata.cloud/) </sup>

2. Large quote with bullets

You can see the info as somebody says on site | documented on the
site  <sup>[1](https://community.netdata.cloud/) </sup>

- Lorem ipsum dolor sit amet, consectetur adipisicing elit, sed do eiusmod tempor incididunt ut
  labore et dolore magna aliqua

- Ut enim ad minim veniam, quis nostrud exercitation ullamco laboris nisi ut aliquip ex ea commodo
  consequat.

- Duis aute irure dolor in reprehenderit in voluptate velit esse cillum dolore eu fugiat nulla
  pariatur.

- Excepteur sint occaecat cupidatat non proident, sunt in culpa qui officia deserunt mollit anim id
  est laborum.

3. Quotes that are just chunks that are 4-10 lines, use block quotes.

> Lorem ipsum dolor sit amet, consectetur adipiscing elit. Nulla convallis lobortis urna eu
sollicitudin. Maecenas vel euismod lacus, vel pulvinar diam. Curabitur luctus metus vitae eros
auctor, et dictum dolor commodo. Vivamus turpis ipsum, placerat et cursus sed, finibus sed velit.
Pellentesque efficitur gravida nibh sit amet lacinia. Vestibulum blandit, enim eu sodales
ullamcorper, dui velit viverra enim, sed fermentum ante orci a risus. Proin cursus, justo id tempor
molestie, dolor ante tempor ante, id dignissim tortor lacus id purus. Nam diam nibh, gravida vitae
sem at, venenatis suscipit ante. Duis quis nulla vel mauris facilisis laoreet et faucibus dui.
Pellentesque quis venenatis est, vitae pharetra leo. <sup>[1](https://community.netdata.cloud/) </sup>

4. For quotes shorter than 4 lines, use quotation marks.

"Lorem ipsum dolor sit amet, consectetur adipisicing elit, sed do eiusmod tempor incididunt ut
labore et dolore magna aliqua" <sup>[1](https://community.netdata.cloud/) </sup>

</details>

### Troubleshooting section:

<details>
<summary>Check these actions regarding to this or that</summary> 
Any independent set of actions/suggestions is explained in a collapsible layout. Here we propose any action the user 
can take.
</details>

<details>
<summary>A second set of actions.</summary> 
Here we propose any action the user can take.
</details>

## OS: <Second_OS_Name>

If the alarm exists in different OS and the underlying mechanisms have major changes or different
troubleshooting sections, we create a section for each OS.

### Troubleshooting section:

And more actions like above.

</details>