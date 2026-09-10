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
	"path/filepath"
	"strconv"

	"github.com/docker/go-units"
	topologyv1 "github.com/netdata/netdata/go/plugins/pkg/topology/v1"
	snmpdiag "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/diagnostics"
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

type openedArchive struct {
	topology diagnosticArchive
	normal   *snmpdiag.NormalDevice
	producer snmpdiag.Producer
}

type archiveOpener func(io.Reader, snmpdiag.ReadLimits) (openedArchive, error)

type commandOptions struct {
	inputPath         string
	normal            bool
	previousRun       bool
	checkpoint        uint64
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
		limits snmpdiag.ReadLimits,
	) (openedArchive, error) {
		return openDiagnosticArchive(reader, limits)
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

	if operation == "list" {
		if options.normal || options.previousRun || options.registrationID != 0 {
			fmt.Fprintln(stderr, "error: list does not support --normal, --previous-run, or --registration-id")
			return 1
		}
		info, err := os.Stat(options.inputPath)
		if err != nil || !info.IsDir() || options.checkpoint != 0 {
			fmt.Fprintln(stderr, "error: list requires a directory and does not select a checkpoint")
			return 1
		}
		entries, err := snmpdiag.ListCheckpoints(options.inputPath)
		if err != nil {
			fmt.Fprintf(stderr, "error: list checkpoints: %v\n", err)
			return 1
		}
		normal, err := snmpdiag.ListNormalFiles(options.inputPath)
		if err != nil {
			fmt.Fprintf(stderr, "error: list normal devices: %v\n", err)
			return 1
		}
		listing := struct {
			Topology []snmpdiag.CheckpointFile `json:"topology_checkpoints"`
			Normal   []snmpdiag.NormalFile     `json:"normal_devices"`
		}{entries, normal}
		if err := jsonv2.MarshalWrite(stdout, listing, outputJSONOptions); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		fmt.Fprintln(stdout)
		return 0
	}
	path, err := resolveCommandInput(options)
	if err != nil {
		fmt.Fprintf(stderr, "error: select input: %v\n", err)
		return 1
	}
	file, err := os.Open(path)
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
	defaults := snmpdiag.DefaultReadLimits()
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
	flags.StringVar(&options.inputPath, "input", "", "new-format diagnostic file or directory (latest topology checkpoint)")
	flags.BoolVar(&options.normal, "normal", false, "select normal device evidence from a directory")
	flags.BoolVar(&options.previousRun, "previous-run", false, "select the previous evidence-bearing normal run")
	flags.Uint64Var(&options.registrationID, "registration-id", 0, "device registration ID")
	flags.Uint64Var(&options.checkpoint, "checkpoint", 0, "checkpoint sequence to select from a directory")
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
	if options.inputPath == "" {
		fmt.Fprintln(stderr, "error: --input is required")
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

func executeOperation(operation string, opened openedArchive, options commandOptions) (any, error) {
	if opened.normal != nil {
		return executeNormalOperation(operation, opened, options)
	}
	archive := opened.topology
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

func readLimits(compressed, decoded string) (snmpdiag.ReadLimits, error) {
	MaxCompressedBytes, err := units.RAMInBytes(compressed)
	if err != nil || MaxCompressedBytes <= 0 {
		return snmpdiag.ReadLimits{}, fmt.Errorf("invalid maximum compressed size %q", compressed)
	}
	MaxDecodedBytes, err := units.RAMInBytes(decoded)
	if err != nil || MaxDecodedBytes <= 0 {
		return snmpdiag.ReadLimits{}, fmt.Errorf("invalid maximum decoded size %q", decoded)
	}
	return snmpdiag.ReadLimits{
		MaxCompressedBytes: MaxCompressedBytes,
		MaxDecodedBytes:    MaxDecodedBytes,
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
	case "list", "validate", "summary", "replay", "inspect-device", "inspect-link":
		return true
	default:
		return false
	}
}

func usage(writer io.Writer) {
	fmt.Fprintln(writer, "usage: snmp-diagnostics <operation> --input PATH [options]")
	fmt.Fprintln(writer, "operations: list, validate, summary, replay, inspect-device, inspect-link")
}

func operationUsage(writer io.Writer, operation string) {
	fmt.Fprintf(writer, "usage: snmp-diagnostics %s --input PATH [options]\n", operation)
}

func openDiagnosticArchive(r io.Reader, limits snmpdiag.ReadLimits) (openedArchive, error) {
	document, err := snmpdiag.Read(r, limits)
	if err != nil {
		return openedArchive{}, err
	}
	if document.Kind == snmpdiag.KindNormal {
		if err := document.Normal.Validate(); err != nil {
			return openedArchive{}, err
		}
		return openedArchive{normal: document.Normal, producer: document.Producer}, nil
	}
	archive, err := snmptopology.InspectDiagnosticDocument(document)
	return openedArchive{topology: archive}, err
}

func resolveInput(input string, sequence uint64) (string, error) {
	info, err := os.Stat(input)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		if sequence != 0 {
			return "", errors.New("--checkpoint requires a directory")
		}
		return input, nil
	}
	entries, err := snmpdiag.ListCheckpoints(input)
	if err != nil {
		return "", err
	}
	if len(entries) == 0 {
		return "", errors.New("directory has no topology checkpoints; select lifecycle.zst for lifecycle inspection")
	}
	if sequence == 0 {
		sequence = entries[len(entries)-1].Sequence
	}
	for _, entry := range entries {
		if entry.Sequence == sequence {
			return filepath.Join(input, snmpdiag.TopologyDirectory, entry.Filename), nil
		}
	}
	return "", fmt.Errorf("checkpoint %d is not retained", sequence)
}
