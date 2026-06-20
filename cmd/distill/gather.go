package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/git-pkgs/enrichment"
	"github.com/git-pkgs/purl"
)

const (
	enrichmentTimeout = 30 * time.Second
	briefTimeout      = 5 * time.Minute
	tmpDirPattern     = "distill-*"
	readmeGlob        = "README*"
)

// gathered is everything distill collects about one target before either
// labeling (classify) or feature extraction (extract) runs on it.
type gathered struct {
	input     string
	purl      string
	repo      string
	dir       string
	briefJSON string
	outline   string
	readme    string
	cleanup   func()
}

type gatherOpts struct {
	briefBin    string
	outlineArgs []string
	keep        bool
}

func gather(enrich enrichment.Client, target string, opts gatherOpts) (*gathered, error) {
	g := &gathered{input: target, cleanup: func() {}}

	source, purlStr, repo := resolveTarget(enrich, target)
	g.purl = purlStr
	g.repo = repo
	if source == "" {
		return g, fmt.Errorf("could not resolve target to a repository")
	}

	dir, err := os.MkdirTemp("", tmpDirPattern)
	if err != nil {
		return g, fmt.Errorf("mkdtemp: %w", err)
	}
	g.dir = dir
	if opts.keep {
		fmt.Fprintf(os.Stderr, "kept: %s -> %s\n", target, dir)
	} else {
		g.cleanup = func() { _ = os.RemoveAll(dir) }
	}

	g.briefJSON, err = runBrief(opts.briefBin, []string{"-json", "-keep", "-dir", dir, source})
	if err != nil {
		return g, fmt.Errorf("brief json: %w", err)
	}

	outlineArgs := append([]string{"outline"}, opts.outlineArgs...)
	outlineArgs = append(outlineArgs, dir)
	g.outline, err = runBrief(opts.briefBin, outlineArgs)
	if err != nil {
		return g, fmt.Errorf("brief outline: %w", err)
	}

	g.readme = readReadme(dir)
	return g, nil
}

// resolveTarget returns (sourceForBrief, purl, repoURL). sourceForBrief is what
// to pass to `brief` (a repo URL or local path); it's empty if unresolvable.
func resolveTarget(enrich enrichment.Client, target string) (source, purlStr, repo string) {
	if strings.HasPrefix(target, "pkg:") {
		purlStr = target
		ctx, cancel := context.WithTimeout(context.Background(), enrichmentTimeout)
		defer cancel()
		info, err := enrich.BulkLookup(ctx, []string{target})
		if err == nil {
			if pi, ok := info[target]; ok && pi.Repository != "" {
				return pi.Repository, purlStr, pi.Repository
			}
		}
		if p, perr := purl.Parse(target); perr == nil {
			if u, uerr := p.RegistryURLWithVersion(); uerr == nil {
				repo = u
			}
		}
		return "", purlStr, repo
	}
	return target, "", target
}

func runBrief(bin string, args []string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), briefTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("%s %s: %w", bin, strings.Join(args, " "), err)
	}
	return string(out), nil
}

func readReadme(dir string) string {
	matches, _ := filepath.Glob(filepath.Join(dir, readmeGlob))
	for _, m := range matches {
		if b, err := os.ReadFile(m); err == nil { //nolint:gosec // dir is our own mkdtemp
			return string(b)
		}
	}
	return ""
}
