package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

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
		fmt.Fprintf(stdout, "config-baseline %s\n", version)
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
			path := finding.Path
			if path == "" {
				path = "/"
			}
			fmt.Fprintf(writer, "%s %s\n", finding.Status, path)
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
