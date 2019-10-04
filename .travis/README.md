# Description of CI build configuration

## Variables needed by travis

- GITHUB_TOKEN - GitHub token with push access to repository
- DOCKER_USERNAME - Username (netdatabot) with write access to docker hub repository
- DOCKER_PWD - Password to docker hub
- encrypted_8daf19481253_key - key needed by openssl to decrypt GCS credentials file
- encrypted_8daf19481253_iv - IV needed by openssl to decrypt GCS credentials file
- COVERITY_SCAN_TOKEN - Token to allow coverity test analysis uploads
- SLACK_USERNAME - This is required for the slack notifications triggered by travis pipeline
- SLACK_CHANNEL - This is the channel that Travis will be posting messages
- SLACK_NOTIFY_WEBHOOK_URL - This is the incoming URL webhook as provided by slack integration. Visit Apps integration in slack to generate the required hook
- SLACK_BOT_NAME - This is the name your bot will appear with on slack
- PKG_CLOUD_TOKEN - This is the token required to access package cloud services
- REPOSITORY - This variable defines the netdata repository and is consumed by the docker build scripts in packaging/docker


## CI workflow details
Our CI pipeline is designed to help us identify and mitigate risks at all stages of implementation.
To accommodate this need, we used [Travis CI](http://www.travis-ci.com) as our CI/CD tool.
Our main areas of concern are:
1) Only push code that is working. That means fail fast so that we can improve before we reach the public

2) Reduce the time to market to minimum, by streamlining the release process.
   That means a lot of testing, a lot of consistency checks, a lot of validations

3) Generated artifacts consistency. We should not allow broken software to reach the public.
   When this happens, it's embarassing and we struggle to eliminate it.

4) We are an innovative company, so we love to automate :)

Having said that, here's a brief introduction to Netdata's improved CI/CD pipeline with Travis.
Our CI/CD lifecycle contains three different execution entry points:
1) A user opens a pull request to netdata/master: Travis will run a pipeline on the branch under that PR
2) A merge or commit happens on netdata/master. This will trigger travis to run, but we have two distinct cases in this scenario:
   a) A user merges a pull request to netdata/master: Travis will run on master, after the merge.
   b) A user runs a commit/merge with a special keyword (mentioned later).
      This triggers a release for either minor, major or release candidate versions or binary package generation, depending the keyword
3) A scheduled job runs on master once per day: Travis will run on master at the scheduled interval

To accommodate all three entry points our CI/CD workflow has a set of steps that run on all three entry points.
Once all these steps are successfull, then our pipeline executes another subset of steps for entry points 2 and 3.
In travis terms the "steps" are "Stages" and within each stage we execute a set of activities called "jobs" in travis.

### Always run: Stages that running on all three execution entry points

## Code quality, linting, syntax, code style
At this early stage we iterate through a set of basic quality control checks:
- Shell checking: Run linters for our various BASH scripts
- Checksum validators: Run validators to ensure our installers and documentation are in sync
- Dashboard validator: We provide a pre-generated dashboard.js script file that we need to make sure its up to date. We validate that.

## Build process
At this stage, basically, we build :-)
We do a baseline check of our build artifacts to guarantee they are not broken
Briefly our activities include:
- Verify docker builds successfully
- Run the standard Netdata installer, to make sure we build & run properly
- Do the same through 'make dist', as this is our stable channel for our kickstart files

## Artifacts validation
At this point we know our software is building, we need to go through the a set of checks, to guarantee
that our product meets certain epxectations. At the current stage, we are focusing on basic capabilities
like installing in different distributions, running the full lifecycle of install-run-update-install and so on.
We are still working on enriching this with more and more use cases, to get us closer to achieving full stability of our software.
Briefly we currently evaluate the following activities:
- Basic software unit testing
- Non containerized build and install on ubuntu 14.04
- Non containerized build and install on ubuntu 18.04
- Running the full Netdata lifecycle (install, update, uninstall) on ubuntu 18.04
- Build and install on CentOS 6
- Build and install on CentOS 7
(More to come)

## Artifacts validation on bare OS, stable to current lifecycle checks
This stage was added to guarantee netdata can be installed, updated, started and uninstalled on all supported operating systems without problems.
We intentionally use clean OS images here, to make sure that dependency management works fine too
This step runs only on pull requests and during nightly cron, to make sure integrity is guaranteed on our code at the critical path (during pull requests or nightly)
 and not mess with other workflows.

### Nightly operations: Stages that run daily under cronjob
The nightly stages are related to the daily nightly activities, that produce our daily latest releases.
We also maintain a couple of cronjobs that run during the night to provide us with deeper insights,
like for example coverity scanning or extended kickstart checksum checks

## Nightly operations
At this stage we run scheduled jobs and execute the nightly changelog generator, coverity scans,
labeler for our issues and extended kickstart files checksum validations.

## Nightly release
During this stage we are building and publishing latest docker images, prepare the nightly artifacts
and deploy them (the artifacts) to our google cloud service provider.


### Publishing
Publishing is responsible for executing the major/minor/patch releases and is separated
in two stages: packaging preparation process and publishing.

## Support activities on main branch
During packaging we are preparing the release changelog information and run the labeler.

## Publish for release
The publishing stage is the most complex part in publishing. This is the stage were we generate and publish docker images,
prepare the release artifacts and get ready with the release draft.

### Package Management workflows
To provide the best support to our customers, we have automated the binary package release process for multiple distributions and architectures.
This gives netdata the ability to provide cutting edge and stable releases through reliable and highly available distribution channels like [package cloud](https://packagecloud.io).

We build for DEB and RPM, across multiple distributions and architectures. The workflows were designed, having in mind that we intend to support all possible platforms on the market, thus we needed to keep things as simple as possible and as flexible as possible.
Package deployment can be triggered manually by executing an empty commit, respecting this message pattern: `[Package PACKAGE_TYPE PACKAGE_ARCH]([Build latest])* DESCRIBE_THE_REASONING_HERE`.
Travis Yaml configuration allows the user to combine package type and architecture as necessary to regenerate the current stable release (For example tag v1.15.0 as of 4th of May 2019).
PACKAGE_TYPE may be either DEB or RPM, while PACKAGE_ARCH at the moment may be either amd64 or i386. If we omit "[Build latest]" string from the commit, then the latest code will be published, otherwise the latest stable tag will be built. Sample patterns to trigger building of packages for all amd64 supported architecture:
- `[Package amd64 RPM] Building latest stable`: Build & publish all available amd64 RPM packages, from latest stable release
- `[Package amd64 DEB] Building latest stable`: Build & publish all available amd64 DEB packages, from latest stable release
- `[Package amd64 DEB][Build latest] Build nightlies`: Build & publish all available amd64 DEB packages, from latest nightly code

### Architectures and Operating systems supported
To find out details about our distribution support, please visit our [distributions page](../packaging/DISTRIBUTIONS.md)

