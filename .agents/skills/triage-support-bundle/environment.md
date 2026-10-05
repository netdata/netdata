# Privileges and the runtime environment

Why the agent cannot read, execute, or see something. These causes produce silence rather than
errors, which is why they are misdiagnosed as collector faults.

Owners: `src/collectors/README.md#collector-privileges` for what each plugin requires and
`src/collectors/README.md#file-permissions-and-ownership` for the expected ownership and modes;
`src/plugins.d/README.md#operation` for how external plugins are launched.

## Privileges

Mode bits alone explain almost nothing. Plugins rely on file capabilities and setuid bits;
distributions apply mandatory access control; packagers use access control lists. None of that shows
in a directory listing, which is exactly why the permissions area captures all of it.

Order of checks:

1. **The plugin directory capture** - mode, ownership, setuid and setgid entries, and per-file
   capabilities. A plugin that lost a capability produces nothing and often logs nothing useful.
2. **The netdata paths capture** - extended attributes, security context, access control lists, and
   non-default filesystem flags on the config, log, state, cache and binary paths. An immutable flag
   on a state directory silently blocks the agent's own writes; a security-context mislabel fails
   collectors with nothing in the agent log.
3. **Mandatory access control state** - whether SELinux or AppArmor is present and enforcing. The
   bundle does **not** carry individual denials or the effective policy, so this indicates that
   mandatory access control may be involved; ask for the audit log when a specific denial has to be
   explained.

Compare against the expected ownership and capabilities in the owner document rather than against
intuition - "root owns it, so it is fine" is wrong here, because several plugins need a specific
group and a specific capability, not root.

Two properties of the capture to respect:

- A configured plugin-directory override is **deliberately not read**, so on a host that relocated
  its plugins the capture may describe a directory the agent does not use. Check the effective
  configuration before concluding the plugins are missing.
- Tools are feature-detected. The extended attribute tool is not installed by default on Debian and
  Ubuntu, so its absence is common and is not a finding. Note the asymmetry: where the filesystem
  flag tool is unavailable those flags are simply omitted, so an empty result is "not checked",
  never "no immutable flag set".

The highest-cost trap in this whole class - capabilities that appear set and do not work - is in
`./false-signals.md`. Read it before reporting a capability as correct.

## Containers and orchestration

Order of checks:

1. **Container context** - the init process, the control groups, and the agent-relevant environment.
   Present only when the host was detected as a container.
2. **Control group version, virtualization detection and the mount table**, which together settle
   whether the agent can see what it is asked to collect. Visibility problems in restricted
   namespaces are a recurring collector-failure class.
3. **Zombie processes**, which indicate plugin reaping failures - the signature of a container
   started without an init process.
4. **Where the logs actually are.** In the official container image the agent's log files are
   symlinks to standard output, so no history exists on disk and the bundle writes a marker naming
   the command that retrieves it from the host. A marker is not logs, and the retrieved output is
   unsanitized, so it must be handled privately. Other images and host-mounted log directories keep
   real files, which the bundle collects normally - check which case you have before sending the
   reporter after container logs that may not exist.

Recurring causes, as patterns: missing host mounts or capabilities; a missing shared process
namespace; socket permission or group-identifier mismatches that surface as unreadable container
names; ownership mismatches between the container user and a bind mount; and discovery gaps in
orchestrated environments.

A note on capabilities in containers: a container that cannot grant file capabilities will still let
the operation appear to succeed. See `./false-signals.md`.

## What the bundle does not settle

- **Service-manager sandboxing on POSIX.** There is no unit or drop-in capture, so a restrictive
  sandbox, a changed user, or a memory policy is invisible except indirectly through permissions and
  log errors. Ask for the effective unit if the evidence points that way. Windows is better covered
  here - it captures the service definition including the account and start mode.
- **Anything about the host outside netdata's own scope** - no full system journal, no other
  services' logs.

## Traps

- "Permission denied" in a log names the path, not the mechanism. The same message results from a
  mode, an access control list, a security context, or a lost capability, and only the permissions
  area separates them.
- Checking the mode but not the group is a common miss, because several plugins depend on group
  ownership rather than on the owner.
- A plugin directory that looks wrong may simply not be the one in use - see the override note above.
