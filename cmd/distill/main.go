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
	case "extract":
		cmdExtract(args[1:])
	case "corpus":
		cmdCorpus(args[1:])
	case "analyse":
		cmdAnalyse(args[1:])
	default:
		usage()
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: distill <command> [args]")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "  classify <purl|url>...   LLM teacher labels packages with oss-taxonomy terms")
	fmt.Fprintln(os.Stderr, "  extract  <purl|url>...   emit deterministic feature record (student input)")
	fmt.Fprintln(os.Stderr, "  corpus   -from <file>    gather once per target, write both labels and features")
	fmt.Fprintln(os.Stderr, "  analyse  [labels.jsonl]  summarise a labels file (coverage, evidence, gaps)")
	os.Exit(exitUsage)
}
