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
				repo = cloneableRepo(pi.Repository)
				return repo, purlStr, repo
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

// cloneableRepo rewrites repository URLs that aren't git-cloneable into a github
// mirror. ecosyste.ms reports some modules with browse-UI URLs that `git clone`
// can't use: golang.org/x/* resolve to cs.opensource.google (or a
// go.googlesource.com remote), and Apache projects resolve to gitbox.apache.org
// gitweb URLs. Prefer the corresponding github.com/golang or github.com/apache
// mirror. Already-cloneable URLs pass through untouched.
func cloneableRepo(repo string) string {
	golangPrefixes := []string{
		"https://cs.opensource.google/go/x/",
		"https://go.googlesource.com/",
		"https://golang.org/x/",
		"http://golang.org/x/",
	}
	for _, p := range golangPrefixes {
		if strings.HasPrefix(repo, p) {
			if name := repoName(strings.TrimPrefix(repo, p)); name != "" {
				return "https://github.com/golang/" + name
			}
		}
	}
	const gitbox = "gitbox.apache.org/repos/asf"
	if i := strings.Index(repo, gitbox); i >= 0 {
		rest := repo[i+len(gitbox):]
		if j := strings.Index(rest, "?p="); j >= 0 {
			rest = rest[j+len("?p="):]
		}
		if name := repoName(strings.TrimPrefix(rest, "/")); name != "" {
			return "https://github.com/apache/" + name
		}
	}
	return repo
}

// repoName takes the first path/query segment of a trailing URL fragment and
// strips a .git suffix, e.g. "crypto/+/refs" -> "crypto", "x.git;a=summary" -> "x".
func repoName(s string) string {
	for _, sep := range []string{"/", "?", ";", "&"} {
		if before, _, found := strings.Cut(s, sep); found {
			s = before
		}
	}
	return strings.TrimSuffix(s, ".git")
}

func runBrief(bin string, args []string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), briefTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = nonInteractiveGitEnv()
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("%s %s: %w", bin, strings.Join(args, " "), err)
	}
	return string(out), nil
}

// nonInteractiveGitEnv returns the process environment with git forced into a
// non-interactive mode with no credential helper, so that cloning a private,
// moved or missing repo fails fast instead of popping a credential dialog.
// brief shells out to system git, which inherits this environment.
func nonInteractiveGitEnv() []string {
	return append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0",
		"GCM_INTERACTIVE=never",
		"SSH_ASKPASS_REQUIRE=never",
		"GIT_SSH_COMMAND=ssh -oBatchMode=yes -oStrictHostKeyChecking=accept-new",
		// Inject `credential.helper=` (git 2.31+) to disable the keychain/GCM
		// helper entirely, so no GUI prompt can appear.
		"GIT_CONFIG_COUNT=1",
		"GIT_CONFIG_KEY_0=credential.helper",
		"GIT_CONFIG_VALUE_0=",
	)
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
