package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/smartcontractkit/chainlink-testing-framework/tools/flakeguard/go-test-transform/pkg/transformer"
)

func main() {
	// Parse command-line flags
	var (
		ignoreAll  bool
		inputFile  string
		outputFile string
	)

	flag.BoolVar(&ignoreAll, "ignore-all", false, "Ignore all subtest failures")
	flag.StringVar(&inputFile, "input", "", "Input JSON file (if not provided, reads from stdin)")
	flag.StringVar(&outputFile, "output", "", "File to write the report to (default: stdout)")
	flag.Parse()

	// Set up options
	opts := transformer.NewOptions(ignoreAll)

	// Determine input source
	var input io.Reader
	if inputFile != "" {
		// Read from file
		file, err := os.Open(inputFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error opening input file: %v\n", err)
			os.Exit(1)
		}
		defer file.Close()
		input = file
	} else {
		// Read from stdin
		input = os.Stdin
	}

	// Determine output destination
	var output io.Writer
	if outputFile != "" {
		// Write to file
		file, err := os.Create(outputFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating output file: %v\n", err)
			os.Exit(1)
		}
		defer file.Close()
		output = file
	} else {
		// Write to stdout
		output = os.Stdout
	}

	// Transform the output
	exitCode, err := transformer.TransformJSON(input, output, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error transforming JSON: %v\n", err)
		os.Exit(1)
	}

	// Exit with the appropriate code
	os.Exit(exitCode)
}
