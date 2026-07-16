# systemd-cat-native

`systemd` includes a utility called `systemd-cat`. This utility reads log lines from its standard input and sends them
to the local systemd journal. Its key limitation is that despite the fact that systemd journals support structured logs,
this command does not support sending structured logs to it.

`systemd-cat-native` is a Netdata supplied utility to push structured logs to systemd journals. Key features:

- reads [Journal Export Format](https://systemd.io/JOURNAL_EXPORT_FORMATS/) formatted log entries
- converts text fields into binary journal multiline log fields
- sends logs to any of these:
  - local default `systemd-journald`,
  - local namespace `systemd-journald`,
  - remote `systemd-journal-remote` using HTTP or HTTPS, the same way `systemd-journal-upload` does.
- is the standard external logger of Netdata shell scripts

## Simple use:

```bash
printf "MESSAGE=hello world\nPRIORITY=6\n\n" | systemd-cat-native
```

The result:

![image](https://github.com/netdata/netdata/assets/2662304/689d5e03-97ee-40a8-a690-82b7710cef7c)


Sending `PRIORITY=3` (error):

```bash
printf "MESSAGE=hey, this is error\nPRIORITY=3\n\n" | systemd-cat-native
```

The result:
![image](https://github.com/netdata/netdata/assets/2662304/faf3eaa5-ac56-415b-9de8-16e6ceed9280)

The program supports multi-line processing for all fields. The default newline sequence is `\n`.

```bash
printf "MESSAGE=hello\\\\nworld\nPRIORITY=6\n\n" | systemd-cat-native --newline='\n'
```

`systemd-cat-native` needs to receive it like this for newline processing to work:

```bash
# printf "MESSAGE=hello\\\\nworld\nPRIORITY=6\n\n"
MESSAGE=hello\nworld
PRIORITY=6

```

It also allows changing the newline sequence. In this example we replace the text `--NEWLINE--` with a newline in the log entry:

```bash
printf "MESSAGE=hello--NEWLINE--world\nPRIORITY=6\n\n" | systemd-cat-native --newline='--NEWLINE--'
```

The result:

![image](https://github.com/netdata/netdata/assets/2662304/d6037b4a-87da-4693-ae67-e07df0decdd9)


## Best practices

These are the rules about fields, enforced by `systemd-journald`:

- field names can be up to **64 characters**,
- field values can be up to **48k characters**,
- the only allowed field characters are **A-Z**, **0-9** and **underscore**,
- the **first** character of fields cannot be a **digit**
- **protected** journal fields start with underscore:
  * they are accepted by `systemd-journal-remote`,
  * they are **NOT** accepted by a local `systemd-journald`.

For best results, always include these fields:

- `MESSAGE=TEXT`<br/>
  The `MESSAGE` is the body of the log entry.
  This field is what we usually see in our logs.

- `PRIORITY=NUMBER`<br/>
  `PRIORITY` sets the severity of the log entry.<br/>
  `0=emerg, 1=alert, 2=crit, 3=err, 4=warn, 5=notice, 6=info, 7=debug`
  - Emergency events (0) are usually broadcast to all terminals.
  - Emergency, alert, critical, and error (0-3) are usually colored red.
  - Warning (4) entries are usually colored yellow.
  - Notice (5) entries are usually bold or have a brighter white color.
  - Info (6) entries are the default.
  - Debug (7) entries are usually grayed or dimmed.

- `SYSLOG_IDENTIFIER=NAME`<br/>
  `SYSLOG_IDENTIFIER` sets the name of application.
  Use something descriptive, like: `SYSLOG_IDENTIFIER=myapp`

You can find the most common fields at `man systemd.journal-fields`.


## Usage

```
Netdata systemd-cat-native v1.43.0-333-g5af71b875

This program reads from its standard input, lines in the format:

KEY1=VALUE1\n
KEY2=VALUE2\n
KEYN=VALUEN\n
\n

and sends them to systemd-journal.

   - Binary journal fields are not accepted at its input
   - Binary journal fields can be generated after newline processing
   - Messages have to be separated by an empty line
   - Keys starting with underscore are not accepted (by journald)
   - Other rules imposed by systemd-journald are imposed (by journald)

Usage:

   systemd-cat-native
          [--newline=STRING]
          [--log-as-netdata|-N]
          [--namespace=NAMESPACE] [--socket=PATH]
          [--url=URL [--key=FILENAME] [--cert=FILENAME] [--trust=FILENAME|all]]

The program has the following modes of logging:

  * Log to a local systemd-journald or stderr

    This is the default mode. If systemd-journald is available, logs will be
    sent to systemd, otherwise logs will be printed on stderr, using logfmt
    formatting. Options --socket and --namespace are available to configure
    the journal destination:

      --socket=PATH
        The path of a systemd-journald UNIX socket.
        The program will use the default systemd-journald socket when this
        option is not used.

      --namespace=NAMESPACE
        The name of a configured and running systemd-journald namespace.
        The program will produce the socket path based on its internal
        defaults, to send the messages to the systemd journal namespace.

  * Log as Netdata, enabled with --log-as-netdata or -N

    In this mode the program uses environment variables set by Netdata for
    the log destination. Only log fields defined by Netdata are accepted.
    If the environment variables expected by Netdata are not found, it
    falls back to stderr logging in logfmt format.

  * Log to a systemd-journal-remote TCP socket, enabled with --url=URL

    In this mode, the program will directly sent logs to a remote systemd
    journal (systemd-journal-remote expected at the destination)
    This mode is available even when the local system does not support
    systemd, or even it is not Linux, allowing a remote Linux systemd
    journald to become the logs database of the local system.

    Unfortunately systemd-journal-remote does not accept compressed
    data over the network, so the stream will be uncompressed.

      --url=URL
        The destination systemd-journal-remote address and port, similarly
        to what /etc/systemd/journal-upload.conf accepts.
        Usually it is in the form: https://ip.address:19532
        Both http and https URLs are accepted. When using https, the
        following additional options are accepted:

      --key=FILENAME
        The filename of the private key of the server.
        The default is: /etc/ssl/private/journal-upload.pem

      --cert=FILENAME
        The filename of the public key of the server.
        The default is: /etc/ssl/certs/journal-upload.pem

      --trust=FILENAME | all
        The filename of the trusted CA public key.
        The default is: /etc/ssl/ca/trusted.pem
        The keyword 'all' can be used to trust all CAs.

      --namespace=NAMESPACE
        Set the namespace of the messages sent.

      --keep-trying
        Keep trying to send the message, if the remote journal is not there.

    NEWLINES PROCESSING
    systemd-journal logs entries may have newlines in them. However the
    Journal Export Format uses binary formatted data to achieve this,
    making it hard for text processing.

    To overcome this limitation, this program allows single-line text
    formatted values at its input, to be binary formatted multi-line Journal
    Export Format at its output.

    To achieve that it allows replacing a given string to a newline.
    The parameter --newline=STRING allows setting the string to be replaced
    with newlines.

    For example by setting --newline='--NEWLINE--', the program will replace
    all occurrences of --NEWLINE-- with the newline character, within each
    VALUE of the KEY=VALUE lines. Once this this done, the program will
    switch the field to the binary Journal Export Format before sending the
    log event to systemd-journal.

```