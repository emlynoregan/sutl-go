package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/emlynoregan/sutl-go"
)

func main() {
	pretty := flag.Bool("pretty", false, "pretty-print the result")
	libraryArg := flag.String("library", "{}", "library map as JSON, @file, or -")
	version := flag.Bool("version", false, "print the package version")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: sutl [flags] <source> <transform>\n")
		fmt.Fprintf(os.Stderr, "Arguments are JSON literals; prefix a path with @ or use - for stdin.\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	if *version {
		fmt.Println("sutl", sutl.Version)
		return
	}
	if flag.NArg() != 2 {
		flag.Usage()
		os.Exit(2)
	}

	source, err := jsonValue(flag.Arg(0), os.Stdin)
	if err != nil {
		fatal(err)
	}
	transform, err := jsonValue(flag.Arg(1), os.Stdin)
	if err != nil {
		fatal(err)
	}
	libraryValue, err := jsonValue(*libraryArg, os.Stdin)
	if err != nil {
		fatal(err)
	}
	libraryMap, ok := sutl.AsMap(libraryValue)
	if !ok {
		fatal(fmt.Errorf("--library must decode to a JSON object"))
	}
	library := sutl.Library(libraryMap)

	result := sutl.Evaluate(source, transform, library)
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetEscapeHTML(false)
	if *pretty {
		encoder.SetIndent("", "  ")
	}
	if err := encoder.Encode(result); err != nil {
		fatal(err)
	}
}

func jsonValue(argument string, stdin io.Reader) (sutl.Value, error) {
	var data []byte
	var err error
	switch {
	case argument == "-":
		data, err = io.ReadAll(stdin)
	case strings.HasPrefix(argument, "@"):
		data, err = os.ReadFile(argument[1:])
	default:
		data = []byte(argument)
	}
	if err != nil {
		return nil, err
	}
	return sutl.DecodeJSON(data)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(2)
}
