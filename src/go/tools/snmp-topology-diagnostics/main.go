// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	jsonv1 "encoding/json"
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/docker/go-units"
	topologyv1 "github.com/netdata/netdata/go/plugins/pkg/topology/v1"
	snmptopology "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology"
)

var outputJSONOptions = jsonv2.JoinOptions(
	jsonv1.DefaultOptionsV1(),
	jsontext.EscapeForHTML(false),
	jsontext.WithIndent("  "),
)

type diagnosticArchive interface {
	Identity() snmptopology.DiagnosticArchiveIdentity
	Summary() (snmptopology.DiagnosticSummary, error)
	Replay(snmptopology.DiagnosticQueryOptions) (topologyv1.Data, error)
	InspectDevice(snmptopology.DiagnosticQueryOptions, uint64) (snmptopology.DiagnosticDeviceInspection, error)
	InspectLink(
		snmptopology.DiagnosticQueryOptions,
		snmptopology.DiagnosticLinkSubject,
	) (snmptopology.DiagnosticLinkInspection, error)
	InspectLinkAt(snmptopology.DiagnosticQueryOptions, int) (snmptopology.DiagnosticLinkInspection, error)
}

type archiveOpener func(io.Reader, snmptopology.DiagnosticReadLimits) (diagnosticArchive, error)

type commandOptions struct {
	archivePath       string
	maxCompressedSize string
	maxDecodedSize    string
	query             snmptopology.DiagnosticQueryOptions
	registrationID    uint64
	link              snmptopology.DiagnosticLinkSubject
	linkIndex         int
	linkIndexSet      bool
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(arguments []string, stdout, stderr io.Writer) int {
	return runWithOpener(arguments, stdout, stderr, func(
		reader io.Reader,
		limits snmptopology.DiagnosticReadLimits,
	) (diagnosticArchive, error) {
		return snmptopology.ReadDiagnosticArchive(reader, limits)
	})
}

func runWithOpener(arguments []string, stdout, stderr io.Writer, openArchive archiveOpener) int {
	if len(arguments) == 0 {
		usage(stderr)
		return 2
	}
	operation := arguments[0]
	if !knownOperation(operation) {
		fmt.Fprintf(stderr, "error: unknown operation %q\n", operation)
		usage(stderr)
		return 2
	}

	options, code := parseCommandOptions(operation, arguments[1:], stderr)
	if code >= 0 {
		return code
	}
	limits, err := readLimits(options.maxCompressedSize, options.maxDecodedSize)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 2
	}

	file, err := os.Open(options.archivePath)
	if err != nil {
		fmt.Fprintf(stderr, "error: open archive: %v\n", err)
		return 1
	}
	defer file.Close()

	archive, err := openArchive(file, limits)
	if err != nil {
		fmt.Fprintf(stderr, "error: read archive: %v\n", err)
		return 1
	}
	result, err := executeOperation(operation, archive, options)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s: %v\n", operation, err)
		return 1
	}
	if err := jsonv2.MarshalWrite(stdout, result, outputJSONOptions); err != nil {
		fmt.Fprintf(stderr, "error: write JSON: %v\n", err)
		return 1
	}
	if _, err := fmt.Fprintln(stdout); err != nil {
		fmt.Fprintf(stderr, "error: write JSON newline: %v\n", err)
		return 1
	}
	return 0
}

func parseCommandOptions(operation string, arguments []string, stderr io.Writer) (commandOptions, int) {
	defaults := snmptopology.DefaultDiagnosticArchiveReadLimits()
	options := commandOptions{
		maxCompressedSize: formatDefaultSize(defaults.MaxCompressedBytes),
		maxDecodedSize:    formatDefaultSize(defaults.MaxDecodedBytes),
		query:             snmptopology.DefaultDiagnosticQueryOptions(),
	}
	flags := flag.NewFlagSet(operation, flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = func() {
		operationUsage(stderr, operation)
		flags.PrintDefaults()
	}
	flags.StringVar(&options.archivePath, "archive", "", "path to a zstd-compressed diagnostic archive")
	flags.StringVar(
		&options.maxCompressedSize,
		"max-compressed-size",
		options.maxCompressedSize,
		"maximum compressed archive size",
	)
	flags.StringVar(
		&options.maxDecodedSize,
		"max-decoded-size",
		options.maxDecodedSize,
		"maximum decoded archive size",
	)
	if operation == "replay" || operation == "inspect-device" || operation == "inspect-link" {
		addQueryFlags(flags, &options.query)
	}
	switch operation {
	case "inspect-device":
		flags.Uint64Var(&options.registrationID, "registration-id", 0, "device registration ID")
	case "inspect-link":
		flags.IntVar(&options.linkIndex, "link-index", -1, "zero-based link index in this archive and query replay")
		flags.StringVar(&options.link.SourceIdentity, "source-identity", "", "source actor identity key")
		flags.StringVar(&options.link.DestinationIdentity, "destination-identity", "", "destination actor identity key")
		flags.StringVar(&options.link.Family, "family", "", "link family")
		flags.StringVar(&options.link.Protocol, "protocol", "", "link protocol (defaults to the family)")
		flags.StringVar(&options.link.Direction, "direction", "", "link direction")
	}
	if err := flags.Parse(arguments); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return commandOptions{}, 0
		}
		return commandOptions{}, 2
	}
	flags.Visit(func(value *flag.Flag) {
		if value.Name == "link-index" {
			options.linkIndexSet = true
		}
	})
	if flags.NArg() != 0 {
		fmt.Fprintf(stderr, "error: unexpected arguments: %v\n", flags.Args())
		return commandOptions{}, 2
	}
	if options.archivePath == "" {
		fmt.Fprintln(stderr, "error: --archive is required")
		return commandOptions{}, 2
	}
	if operation == "inspect-device" && options.registrationID == 0 {
		fmt.Fprintln(stderr, "error: --registration-id must be greater than zero")
		return commandOptions{}, 2
	}
	if operation == "inspect-link" {
		hasCompositeSelector := options.link.SourceIdentity != "" || options.link.DestinationIdentity != "" ||
			options.link.Family != "" || options.link.Protocol != "" || options.link.Direction != ""
		if options.linkIndexSet && hasCompositeSelector {
			fmt.Fprintln(stderr, "error: --link-index and identity-based link selectors are mutually exclusive")
			return commandOptions{}, 2
		}
		if options.linkIndexSet {
			if options.linkIndex < 0 {
				fmt.Fprintln(stderr, "error: --link-index must be zero or greater")
				return commandOptions{}, 2
			}
		} else if options.link.SourceIdentity == "" || options.link.DestinationIdentity == "" ||
			options.link.Family == "" || options.link.Direction == "" {
			fmt.Fprintln(
				stderr,
				"error: a link selector is required: use --link-index or --source-identity, "+
					"--destination-identity, --family, and --direction",
			)
			return commandOptions{}, 2
		}
	}
	return options, -1
}

func addQueryFlags(flags *flag.FlagSet, options *snmptopology.DiagnosticQueryOptions) {
	flags.BoolVar(
		&options.CollapseActorsByIP,
		"collapse-actors-by-ip",
		options.CollapseActorsByIP,
		"collapse topology actors by IP address",
	)
	flags.BoolVar(
		&options.EliminateNonIPInferred,
		"eliminate-non-ip-inferred",
		options.EliminateNonIPInferred,
		"remove inferred actors without an IP address",
	)
	flags.StringVar(&options.MapType, "map-type", options.MapType, "topology map type")
	flags.StringVar(
		&options.InferenceStrategy,
		"inference-strategy",
		options.InferenceStrategy,
		"topology inference strategy",
	)
	flags.StringVar(
		&options.ManagedDeviceFocus,
		"managed-device-focus",
		options.ManagedDeviceFocus,
		"managed device focus selector",
	)
	flags.StringVar(&options.Depth, "depth", options.Depth, "topology traversal depth or all")
}

func executeOperation(operation string, archive diagnosticArchive, options commandOptions) (any, error) {
	switch operation {
	case "validate":
		return snmptopology.DiagnosticValidation{Valid: true, Archive: archive.Identity()}, nil
	case "summary":
		return archive.Summary()
	case "replay":
		return archive.Replay(options.query)
	case "inspect-device":
		return archive.InspectDevice(options.query, options.registrationID)
	case "inspect-link":
		if options.linkIndexSet {
			return archive.InspectLinkAt(options.query, options.linkIndex)
		}
		return archive.InspectLink(options.query, options.link)
	default:
		return nil, fmt.Errorf("unknown operation %q", operation)
	}
}

func readLimits(compressed, decoded string) (snmptopology.DiagnosticReadLimits, error) {
	maxCompressedBytes, err := units.RAMInBytes(compressed)
	if err != nil || maxCompressedBytes <= 0 {
		return snmptopology.DiagnosticReadLimits{}, fmt.Errorf("invalid maximum compressed size %q", compressed)
	}
	maxDecodedBytes, err := units.RAMInBytes(decoded)
	if err != nil || maxDecodedBytes <= 0 {
		return snmptopology.DiagnosticReadLimits{}, fmt.Errorf("invalid maximum decoded size %q", decoded)
	}
	return snmptopology.DiagnosticReadLimits{
		MaxCompressedBytes: maxCompressedBytes,
		MaxDecodedBytes:    maxDecodedBytes,
	}, nil
}

func formatDefaultSize(bytes int64) string {
	const mebibyte = int64(1 << 20)
	if bytes > 0 && bytes%mebibyte == 0 {
		return strconv.FormatInt(bytes/mebibyte, 10) + "MiB"
	}
	return strconv.FormatInt(bytes, 10) + "B"
}

func knownOperation(operation string) bool {
	switch operation {
	case "validate", "summary", "replay", "inspect-device", "inspect-link":
		return true
	default:
		return false
	}
}

func usage(writer io.Writer) {
	fmt.Fprintln(writer, "usage: snmp-topology-diagnostics <operation> --archive PATH [options]")
	fmt.Fprintln(writer, "operations: validate, summary, replay, inspect-device, inspect-link")
}

func operationUsage(writer io.Writer, operation string) {
	fmt.Fprintf(writer, "usage: snmp-topology-diagnostics %s --archive PATH [options]\n", operation)
}
