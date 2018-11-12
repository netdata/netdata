# Contributing

Thank you for considering contributing to Netdata.

We love to receive contributions. Maintaining a platform for monitoring everything imaginable requires a broad understanding of a plethora of technologies, systems and applications. We rely on community contributions and user feedback to continue providing the best monitoring solution out there.

There are many ways to contribute, with varying requirements of skills:

## All NetData Users

### Give Netdata a GitHub star

This is the minimum open-source users should contribute back to the projects they use. Github stars help the project gain visibility, stand out. So, if you use Netdata, consider pressing that button. **It really matters**.

### Spread the word

Community growth allows the project to attract new talent willing to contribute. This talent is then developing new features and improves the project. These new features and improvements attract more users and so on. It is a loop. So, post about netdata, present it to local meetups you attend, let your online social network or twitter, facebook, reddit, etc. know you are using it. **The more people involved, the faster the project evolves**.

### Provide feedback

Is there anything that bothers you about netdata? Did you experience an issue while installing it or using it? Would you like to see it evolve to you need? Let us know. [Open a github issue](https://github.com/netdata/netdata/issues) to discuss it. Feedback is very important for open-source projects. We can't commit we will do everything, but your feedback influences our road-map significantly. **We rely on your feedback to make Netdata better**.

#### Help the developers understand what they have to do

NetData is all about simplicity and meaningful presentation. It's impossible for a handful of people to know which metrics really matter when monitoring a particular software or hardware component you are interested in. Be specific about what should be collected, how the information should be presented in the dashboard and which alarms make sense in most situations.

## Experienced Users

### Help other users

As the project grows, an increasing share of our time is spent on supporting this community of users in terms of answering questions, of helping users understand how netdata works and find their way with it. Helping other users is crucial. It allows the developers and maintainers of the project to focus on improving it.

### Improve documentation

Most of our documentation is in markdown (.md) files inside the netdata GitHub project. What remains in our Wiki will soon be moved in there as well. Don't be afraid to edit any of these documents and submit a GitHub Pull Request with your corrections/additions. 


## Developers

We expect most contributions to be for new data collection plugins. You can read about how external plugins work [here](collectors/plugins.d/). Additional instructions are available for [Node.js plugins](collectors/node.d/) and [Python plugis](collectors/python.d/).

Of course we appreciate contributions for any other part of the NetData agent, including the [deamon](deamon/), [backends for long term archiving](backends/), innovative ways of using the [REST API](web/api) to create cool [Custom Dashboards](web/gui/custom/) or to include NetData charts in other applications, similarly to what can be done with [Confluence](web/gui/confluence/).


### Contributions Ground Rules

#### Code of Conduct and CLA

We expect all contributors to abide by the [Contributor Covenant Code of Conduct](CODE_OF_CONDUCT.md). For a pull request to be accepted, you will also need to accept the [netdata contributors license agreement](CONTRIBUTORS.md), as part of the PR process.

#### Performance and efficiency

Everything on Netdata is about efficiency. We need netdata to always be the most lightweight monitoring solution available. We will reject to merge PRs that are not optimal in resource utilization and efficiency.

Of course there are cases that such technical excellence is either not reasonable or not feasible. In these cases, we may require the feature or code submitted to be by disabled by default.

#### Meaningful metrics

Unlike other monitoring solutions, Netdata requires all metrics collected to have some structure attached to them. So, Netdata metrics have a name, units, belong to a chart that has a title, a family, a context, belong to an application, etc.

This structure is what makes netdata different. Most other monitoring solution collect bulk metrics in terms of name-value pairs and then expect their users to give meaning to these metrics during visualization. This does not work. It is neither practical nor reasonable to give to someone 2000 metrics and let him/her visualize them in a meaningful way.

So, netdata requires all metrics to have a meaning at the time they are collected.  We will reject to merge PRs that loosely collect just a "bunch of metrics", but we are very keen to help you fix this.

#### Automated Testing

Netdata is a very large application to have automated testing end-to-end. But automated testing is required for crucial functions of it.

Generally, all pull requests should be coupled with automated testing scenarios. However since we do not currently have a framework in place for testing everything little bit of it, we currently require automated tests for parts of Netdata that seem too risky to be changed without automated testing.

Of course, manual testing is always required.

#### Netdata is a distributed application

Netdata is a distributed monitoring application. A few basic features can become quite complicated for such applications. We may reject features that alter or influence the nature of netdata, though we usually discuss the requirements with contributors and help them adapt their code to be better suited for Netdata.

#### Operating systems supported

Netdata should be running everywhere, on every production system out there.

Although we focus on **supported operating systems**, we still want Netdata to run even on non-supported systems. This, of course, may require some more manual work from the users (to prepare their environment, or enable certain flags, etc).

If your contributions limit the number of operating systems supported we will request from you to improve it.

#### Documentation

Your contributions should be bundled with related documentation to help users understand how to use the features you introduce.

#### Maintenance

When you contribute code to Netdata, you are automatically accepting that you will be responsible for maintaining that code in the future. So, if users need help, or report bugs, we will invite you to the related github issues to help them or fix the issues or bugs of your contributions.

