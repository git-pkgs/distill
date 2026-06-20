// Command gencorpus builds the training corpus seed. It fetches the most-
// depended-on packages per registry from packages.ecosyste.ms (corpus/top-deps.txt)
// and merges them with the hand-curated knowledge-base repo list
// (corpus/knowledge.txt) into corpus/seed.txt, deduplicated on repo URL.
//
//	go run ./tools/gencorpus            fetch top-deps then rebuild seed
//	go run ./tools/gencorpus -fetch=false   rebuild seed from existing top-deps.txt
//
// dependent_repos_count, not dependent_packages_count: the latter is trivially
// gamed by publishing many packages that all depend on each other.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

var registries = []string{
	"npmjs.org", "pypi.org", "rubygems.org", "crates.io", "proxy.golang.org",
	"repo1.maven.org", "packagist.org", "nuget.org", "hex.pm", "pub.dev",
}

const (
	apiBase     = "https://packages.ecosyste.ms/api/v1/registries"
	httpTimeout = 30 * time.Second
	defaultTop  = 50
	overFetch   = 2 // fetch 2x and keep the first N with a repo URL
	filePerm    = 0o644
)

func main() {
	perRegistry := flag.Int("top", defaultTop, "Packages to keep per registry")
	fetch := flag.Bool("fetch", true, "Fetch top-deps from the API (false = reuse top-deps.txt)")
	dir := flag.String("dir", "corpus", "Corpus directory")
	flag.Parse()

	if err := run(*dir, *perRegistry, *fetch); err != nil {
		fmt.Fprintf(os.Stderr, "gencorpus: %v\n", err)
		os.Exit(1)
	}
}

func run(dir string, perRegistry int, fetch bool) error {
	if fetch {
		if err := genTopDeps(dir, perRegistry); err != nil {
			return err
		}
	}
	return genSeed(dir)
}

type pkg struct {
	Purl          string `json:"purl"`
	RepositoryURL string `json:"repository_url"`
}

func genTopDeps(dir string, perRegistry int) error {
	var b strings.Builder
	fmt.Fprintf(&b, "# Top-%d packages per registry by dependent_repos_count from packages.ecosyste.ms.\n", perRegistry)
	fmt.Fprintf(&b, "# Generated %s. Format: purl<TAB>repo_url\n\n", time.Now().UTC().Format("2006-01-02"))

	client := &http.Client{Timeout: httpTimeout}
	for _, reg := range registries {
		fmt.Fprintf(os.Stderr, "fetching %s...\n", reg)
		pkgs, err := fetchTop(client, reg, perRegistry*overFetch)
		if err != nil {
			fmt.Fprintf(os.Stderr, "skip %s: %v\n", reg, err)
			continue
		}
		fmt.Fprintf(&b, "# %s\n", reg)
		kept := 0
		for _, p := range pkgs {
			// distill needs source to clone; nuget in particular has many
			// repo-less entries, so over-fetch and keep the first N with a repo.
			if p.Purl == "" || p.RepositoryURL == "" {
				continue
			}
			fmt.Fprintf(&b, "%s\t%s\n", p.Purl, p.RepositoryURL)
			if kept++; kept >= perRegistry {
				break
			}
		}
		b.WriteByte('\n')
	}
	return os.WriteFile(filepath.Join(dir, "top-deps.txt"), []byte(b.String()), filePerm) //nolint:gosec
}

func fetchTop(client *http.Client, registry string, n int) ([]pkg, error) {
	url := fmt.Sprintf("%s/%s/packages?sort=dependent_repos_count&order=desc&per_page=%d", apiBase, registry, n)
	resp, err := client.Get(url) //nolint:gosec,noctx // operator tool, fixed host
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}
	var pkgs []pkg
	if err := json.NewDecoder(resp.Body).Decode(&pkgs); err != nil {
		return nil, err
	}
	return pkgs, nil
}

// genSeed merges top-deps.txt (purl<TAB>repo, purl preferred) with knowledge.txt
// (bare repo URLs), deduped on normalised repo URL.
func genSeed(dir string) error {
	byRepo := map[string]string{} // normalised repo -> preferred identifier
	var order []string
	add := func(key, id string) {
		if _, ok := byRepo[key]; !ok {
			order = append(order, key)
			byRepo[key] = id
		}
	}

	topDeps, err := readNonComment(filepath.Join(dir, "top-deps.txt"))
	if err != nil {
		return err
	}
	for _, line := range topDeps {
		purl, repo, _ := strings.Cut(line, "\t")
		key := normRepo(repo)
		if key == "" {
			key = purl
		}
		add(key, purl)
	}

	knowledge, err := readNonComment(filepath.Join(dir, "knowledge.txt"))
	if err != nil {
		return err
	}
	for _, url := range knowledge {
		add(normRepo(url), url)
	}

	ids := make([]string, 0, len(order))
	for _, k := range order {
		ids = append(ids, byRepo[k])
	}
	sort.Strings(ids)

	var b strings.Builder
	fmt.Fprintf(&b, "# Merged corpus seed: top-deps.txt + knowledge.txt, deduped on repo URL.\n")
	fmt.Fprintf(&b, "# Generated %s. %d entries.\n", time.Now().UTC().Format("2006-01-02"), len(ids))
	b.WriteString("# examples.txt excluded (unresolved bare names).\n\n")
	for _, id := range ids {
		b.WriteString(id)
		b.WriteByte('\n')
	}
	if err := os.WriteFile(filepath.Join(dir, "seed.txt"), []byte(b.String()), filePerm); err != nil { //nolint:gosec
		return err
	}
	fmt.Fprintf(os.Stderr, "gencorpus: wrote %d entries to %s/seed.txt\n", len(ids), dir)
	return nil
}

func normRepo(url string) string {
	u := strings.TrimSpace(strings.ToLower(url))
	u = strings.TrimSuffix(u, ".git")
	return strings.TrimSuffix(u, "/")
}

func readNonComment(path string) ([]string, error) {
	data, err := os.ReadFile(path) //nolint:gosec // operator-supplied corpus path
	if err != nil {
		return nil, err
	}
	var out []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, line)
	}
	return out, nil
}
