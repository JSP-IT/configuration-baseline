package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"strconv"
	"unicode"

	"github.com/JSP-IT/configuration-baseline/internal/baseline"
)

var version = "dev"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("config-baseline", flag.ContinueOnError)
	flags.SetOutput(stderr)

	baselinePath := flags.String("baseline", "", "path to the approved baseline (.json, .yaml, or .yml)")
	configPath := flags.String("config", "", "path to the configuration to inspect (.json, .yaml, or .yml)")
	outputFormat := flags.String("format", "text", "report format: text or json")
	showVersion := flags.Bool("version", false, "print the version and exit")
	flags.Usage = func() {
		fmt.Fprintln(stderr, "Usage: config-baseline -baseline FILE -config FILE [-format text|json]")
		fmt.Fprintln(stderr, "       config-baseline -version")
		fmt.Fprintln(stderr, "\nCompare a configuration with an approved baseline without modifying either file.")
		flags.PrintDefaults()
	}

	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(stderr, "error: positional arguments are not supported")
		return 2
	}
	if *showVersion {
		fmt.Fprintf(stdout, "config-baseline %s\n", versionString())
		return 0
	}
	if *baselinePath == "" || *configPath == "" {
		fmt.Fprintln(stderr, "error: both -baseline and -config are required")
		flags.Usage()
		return 2
	}
	if *outputFormat != "text" && *outputFormat != "json" {
		fmt.Fprintln(stderr, "error: -format must be text or json")
		return 2
	}

	document, err := baseline.LoadBaseline(*baselinePath)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 2
	}
	config, err := baseline.LoadConfig(*configPath)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 2
	}

	report := baseline.Compare(document, config)
	if *outputFormat == "json" {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(report); err != nil {
			fmt.Fprintln(stderr, "error: could not write JSON report")
			return 2
		}
	} else {
		writeTextReport(stdout, report)
	}

	if report.Summary.Total > 0 {
		return 1
	}
	return 0
}

func writeTextReport(writer io.Writer, report baseline.Report) {
	if report.Summary.Total == 0 {
		fmt.Fprintln(writer, "Configuration matches baseline.")
	} else {
		fmt.Fprintln(writer, "Configuration drift detected.")
		for _, finding := range report.Findings {
			fmt.Fprintf(writer, "%s %s\n", finding.Status, displayPath(finding.Path))
		}
	}
	fmt.Fprintf(
		writer,
		"Summary: %d missing, %d unexpected, %d modified (%d total)\n",
		report.Summary.Missing,
		report.Summary.Unexpected,
		report.Summary.Modified,
		report.Summary.Total,
	)
}

// displayPath formats a JSON Pointer for the line-oriented text report. The
// whole document (the empty pointer) is shown as "(root)" so it cannot be
// confused with "/", the pointer to an empty key. Pointers containing control
// or other non-printable characters are quoted so that crafted keys cannot
// forge additional report lines.
func displayPath(path string) string {
	if path == "" {
		return "(root)"
	}
	for _, r := range path {
		if !unicode.IsPrint(r) {
			return strconv.Quote(path)
		}
	}
	return path
}

func versionString() string {
	if version != "dev" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return version
}
