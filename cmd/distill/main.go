package main

import (
	"fmt"
	"os"
)

const exitUsage = 2

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		usage()
	}
	switch args[0] {
	case "classify":
		cmdClassify(args[1:])
	default:
		usage()
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: distill <command> [args]")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "  classify <purl|url>...   classify packages into oss-taxonomy terms")
	os.Exit(exitUsage)
}
