package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/git-pkgs/enrichment"
)

// cmdCorpus runs gather() once per target and emits both the feature record and
// the teacher label, so a full corpus run clones each repo once instead of twice.
// Output files are opened append and previously-labelled inputs are skipped, so
// an interrupted run can be restarted with the same arguments.
func cmdCorpus(args []string) { os.Exit(runCorpus(args)) }

func runCorpus(args []string) int {
	fs := flag.NewFlagSet("distill corpus", flag.ExitOnError)
	from := fs.String("from", "", "Read targets from file (one per line, # comments)")
	labelsPath := fs.String("labels", "labels.jsonl", "Append teacher labels here")
	featuresPath := fs.String("features", "features.jsonl", "Append feature records here")
	model := fs.String("model", defaultModel, "Claude model ID")
	briefBin := fs.String("brief", "brief", "Path to brief binary")
	claudeBin := fs.String("claude", "claude", "Path to claude CLI")
	maxFiles := fs.Int("max-files", 0, "Pass through to brief outline -max-files (0 = outline default)")
	outlineCap := fs.Int("outline-cap", defaultOutlineCap, "Max bytes of outline in teacher prompt")
	readmeCap := fs.Int("readme-cap", defaultReadmeCap, "Max bytes of README in teacher prompt")
	maxIdents := fs.Int("max-identifiers", defaultMaxIdentifiers, "Cap on distinct identifiers per repo")
	keep := fs.Bool("keep", false, "Keep cloned source directories")
	_ = fs.Parse(args)

	targets, err := loadTargets(*from, fs.Args())
	if err != nil {
		return errorf("%v", err)
	}
	if len(targets) == 0 {
		fmt.Fprintln(os.Stderr, "error: no targets (use -from <file> or pass purls/urls)")
		return exitUsage
	}

	done := alreadyLabelled(*labelsPath)
	labels, err := openAppend(*labelsPath)
	if err != nil {
		return errorf("open %s: %v", *labelsPath, err)
	}
	defer func() { _ = labels.Close() }()
	features, err := openAppend(*featuresPath)
	if err != nil {
		return errorf("open %s: %v", *featuresPath, err)
	}
	defer func() { _ = features.Close() }()

	enrich, err := enrichment.NewClient(enrichment.WithUserAgent("git-pkgs/distill"))
	if err != nil {
		return errorf("enrichment client: %v", err)
	}

	gopts := gatherOpts{briefBin: *briefBin, keep: *keep}
	if *maxFiles > 0 {
		gopts.outlineArgs = []string{"-max-files", fmt.Sprint(*maxFiles)}
	}
	copts := classifyOpts{
		model:      *model,
		briefBin:   *briefBin,
		claudeBin:  *claudeBin,
		outlineCap: *outlineCap,
		readmeCap:  *readmeCap,
		keep:       *keep,
	}

	lEnc := json.NewEncoder(labels)
	fEnc := json.NewEncoder(features)
	exit := 0
	for i, t := range targets {
		if done[t] {
			fmt.Fprintf(os.Stderr, "[%d/%d] %s: already labelled, skipping\n", i+1, len(targets), t)
			continue
		}
		fmt.Fprintf(os.Stderr, "[%d/%d] %s\n", i+1, len(targets), t)
		g, gerr := gather(enrich, t, gopts)
		feat := featuresFrom(g, gerr, *maxIdents)
		lab := classifyFrom(g, gerr, copts)
		g.cleanup()
		if feat.Error != "" || lab.Error != "" {
			exit = 1
		}
		_ = fEnc.Encode(feat)
		_ = lEnc.Encode(lab)
	}
	return exit
}

func errorf(format string, a ...any) int {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", a...)
	return 1
}

func loadTargets(from string, argv []string) ([]string, error) {
	seen := map[string]bool{}
	var out []string
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" || strings.HasPrefix(s, "#") || seen[s] {
			return
		}
		// corpus files may be "purl<TAB>repo_url"; take the first field.
		if i := strings.IndexAny(s, "\t "); i > 0 {
			s = s[:i]
		}
		seen[s] = true
		out = append(out, s)
	}
	if from != "" {
		f, err := os.Open(from) //nolint:gosec // user-supplied path is the point
		if err != nil {
			return nil, err
		}
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			add(sc.Text())
		}
		_ = f.Close()
		if err := sc.Err(); err != nil {
			return nil, err
		}
	}
	for _, a := range argv {
		add(a)
	}
	return out, nil
}

// alreadyLabelled returns the set of .input values present in an existing
// labels file, so a restarted run skips work already done.
func alreadyLabelled(path string) map[string]bool {
	done := map[string]bool{}
	f, err := os.Open(path) //nolint:gosec // user-supplied path is the point
	if err != nil {
		return done
	}
	defer func() { _ = f.Close() }()
	const maxLine = 1 << 20 // labels lines can be long (evidence text)
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0), maxLine)
	for sc.Scan() {
		var r struct {
			Input string `json:"input"`
			Error string `json:"error"`
		}
		if json.Unmarshal(sc.Bytes(), &r) == nil && r.Input != "" && r.Error == "" {
			done[r.Input] = true
		}
	}
	return done
}

func openAppend(path string) (*os.File, error) {
	const perm = 0o644
	return os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, perm) //nolint:gosec
}
