#!/usr/bin/env python3
"""
Estimate how much bandwidth Docker Hub's Official Images (the `library/`
namespace) has served, as a proxy for the infrastructure subsidy Docker
provides to the open source ecosystem.

Pipeline:
  1. Enumerate every repo in library/ via the Hub v2 API (paginated).
  2. For each repo, fetch the compressed image size from the tags endpoint:
     use the `latest` tag's full_size if present, else the median full_size
     across the first page of tags.
  3. estimated_bytes = pull_count * full_size; aggregate and rank.

Outputs:
  docker_official_images.csv  - one row per repo
  summary.md                  - methodology, totals, top-50, concentration,
                                caveats
Headline numbers are printed to stdout.

No external deps beyond `requests`. Raw JSON responses are cached under
./cache/ so re-runs don't re-fetch.
"""

import csv
import json
import os
import statistics
import sys
import time
from urllib.parse import urlencode

import requests

BASE = "https://hub.docker.com/v2"
NAMESPACE = "library"
CACHE_DIR = os.path.join(os.path.dirname(os.path.abspath(__file__)), "cache")
USER_AGENT = "official-images-bandwidth-analysis/1.0 (research; polite)"

# Pricing assumptions for egress, dollars per GB.
COST_LOW = 0.02   # Cloudflare-ish / bandwidth alliance
COST_HIGH = 0.09  # AWS list-price egress

GB = 1_000_000_000        # decimal GB, matches how egress is billed
TB = 1_000_000_000_000

MIN_INTERVAL = 1.0        # seconds between live requests (~1 req/sec)
MAX_RETRIES = 5

_last_request = 0.0
_session = requests.Session()
_session.headers.update({"User-Agent": USER_AGENT, "Accept": "application/json"})


def _throttle():
    """Block so that live requests are spaced at most ~1/sec apart."""
    global _last_request
    wait = MIN_INTERVAL - (time.time() - _last_request)
    if wait > 0:
        time.sleep(wait)
    _last_request = time.time()


def _cache_path(key):
    safe = key.replace("/", "_").replace("?", "_").replace("&", "_").replace("=", "_")
    return os.path.join(CACHE_DIR, safe + ".json")


def fetch(url, cache_key):
    """GET url as JSON, caching to disk under cache_key. Retries on 429/5xx."""
    path = _cache_path(cache_key)
    if os.path.exists(path):
        with open(path) as f:
            return json.load(f)

    backoff = 2.0
    for attempt in range(1, MAX_RETRIES + 1):
        _throttle()
        try:
            resp = _session.get(url, timeout=30)
        except requests.RequestException as e:
            if attempt == MAX_RETRIES:
                raise
            sys.stderr.write(f"  request error ({e}); retry {attempt} in {backoff:.0f}s\n")
            time.sleep(backoff)
            backoff *= 2
            continue

        if resp.status_code == 200:
            data = resp.json()
            os.makedirs(CACHE_DIR, exist_ok=True)
            with open(path, "w") as f:
                json.dump(data, f)
            return data

        if resp.status_code == 429 or resp.status_code >= 500:
            if attempt == MAX_RETRIES:
                resp.raise_for_status()
            retry_after = resp.headers.get("Retry-After")
            sleep_for = float(retry_after) if retry_after and retry_after.isdigit() else backoff
            sys.stderr.write(
                f"  HTTP {resp.status_code}; retry {attempt} in {sleep_for:.0f}s\n"
            )
            time.sleep(sleep_for)
            backoff *= 2
            continue

        # 404 or other client error: don't retry, surface as None upstream.
        resp.raise_for_status()

    raise RuntimeError(f"exhausted retries for {url}")


def enumerate_repos():
    """Return list of repo dicts across all pages of the library/ namespace."""
    repos = []
    page = 1
    while True:
        qs = urlencode({"page_size": 100, "page": page})
        url = f"{BASE}/repositories/{NAMESPACE}/?{qs}"
        data = fetch(url, f"repos_page_{page}")
        results = data.get("results", [])
        repos.extend(results)
        sys.stderr.write(f"repos: page {page}, +{len(results)} (total {len(repos)})\n")
        if not data.get("next"):
            break
        page += 1
    return repos


def repo_size(name):
    """
    Return (full_size_bytes, source) for a repo.

    source is 'latest' if the `latest` tag was found, else 'median' over the
    first page of tags, else 'none' if no sized tag exists.
    """
    qs = urlencode({"page_size": 25})
    url = f"{BASE}/repositories/{NAMESPACE}/{name}/tags/?{qs}"
    try:
        data = fetch(url, f"tags_{name}")
    except requests.HTTPError as e:
        sys.stderr.write(f"  tags fetch failed for {name}: {e}\n")
        return None, "none"

    results = data.get("results", [])
    sizes = [r["full_size"] for r in results
             if isinstance(r.get("full_size"), int) and r["full_size"] > 0]

    for r in results:
        if r.get("name") == "latest" and isinstance(r.get("full_size"), int) \
                and r["full_size"] > 0:
            return r["full_size"], "latest"

    if sizes:
        return int(statistics.median(sizes)), "median"
    return None, "none"


def main():
    repos = enumerate_repos()
    rows = []
    for i, repo in enumerate(repos, 1):
        name = repo["name"]
        size, source = repo_size(name)
        pull_count = repo.get("pull_count") or 0
        est_bytes = pull_count * size if size else 0
        rows.append({
            "name": name,
            "pull_count": pull_count,
            "star_count": repo.get("star_count") or 0,
            "full_size": size or 0,
            "size_source": source,
            "est_bytes": est_bytes,
            "date_registered": repo.get("date_registered") or "",
            "last_updated": repo.get("last_updated") or "",
        })
        sys.stderr.write(
            f"[{i}/{len(repos)}] {name}: pulls={pull_count:,} "
            f"size={(size or 0)/1e6:.1f}MB ({source})\n"
        )

    write_csv(rows)
    write_summary(rows)
    print_headline(rows)


def write_csv(rows):
    out = os.path.join(os.path.dirname(os.path.abspath(__file__)),
                       "docker_official_images.csv")
    with open(out, "w", newline="") as f:
        w = csv.writer(f)
        w.writerow([
            "name", "pull_count", "size_mb", "size_source", "est_tb",
            "est_cost_low", "est_cost_high", "date_registered",
        ])
        for r in sorted(rows, key=lambda x: x["est_bytes"], reverse=True):
            est_tb = r["est_bytes"] / TB
            gb = r["est_bytes"] / GB
            w.writerow([
                r["name"],
                r["pull_count"],
                round(r["full_size"] / 1e6, 3),
                r["size_source"],
                round(est_tb, 4),
                round(gb * COST_LOW, 2),
                round(gb * COST_HIGH, 2),
                r["date_registered"],
            ])
    sys.stderr.write(f"wrote {out}\n")


def _fmt_bytes(b):
    """Human-readable decimal scale."""
    for unit, scale in (("PB", 1e15), ("TB", 1e12), ("GB", 1e9), ("MB", 1e6)):
        if b >= scale:
            return f"{b / scale:.2f} {unit}"
    return f"{b:.0f} B"


def write_summary(rows):
    total_pulls = sum(r["pull_count"] for r in rows)
    total_bytes = sum(r["est_bytes"] for r in rows)
    total_gb = total_bytes / GB
    n_latest = sum(1 for r in rows if r["size_source"] == "latest")
    n_median = sum(1 for r in rows if r["size_source"] == "median")
    n_none = sum(1 for r in rows if r["size_source"] == "none")

    by_bytes = sorted(rows, key=lambda x: x["est_bytes"], reverse=True)
    by_pulls = sorted(rows, key=lambda x: x["pull_count"], reverse=True)

    top10_bytes = sum(r["est_bytes"] for r in by_bytes[:10])
    top50_bytes = sum(r["est_bytes"] for r in by_bytes[:50])
    share10 = (top10_bytes / total_bytes * 100) if total_bytes else 0
    share50 = (top50_bytes / total_bytes * 100) if total_bytes else 0

    out = os.path.join(os.path.dirname(os.path.abspath(__file__)), "summary.md")
    with open(out, "w") as f:
        f.write("# Docker Hub Official Images: estimated bandwidth served\n\n")
        f.write(f"_Generated from the Docker Hub v2 API. {len(rows)} repositories "
                f"in the `library/` namespace._\n\n")

        f.write("## Methodology\n\n")
        f.write(
            "Every repository in Docker Hub's `library/` namespace (the curated "
            "**Official Images** programme) was enumerated through the public "
            "`/v2/repositories/library/` API, capturing each repo's lifetime "
            "`pull_count`. For each repo the compressed image size was taken from "
            "the tags endpoint: the `full_size` of the `latest` tag where it "
            "exists, otherwise the median `full_size` across the first page of "
            "tags. Estimated bytes served per repo is simply "
            "`pull_count x full_size`; these are summed across the namespace and "
            "priced at $%.2f/GB (Cloudflare-ish bandwidth-alliance rate) and "
            "$%.2f/GB (AWS list-price egress). Sizes are decimal GB/TB, matching "
            "how egress is metered. Of %d repos, %d used the `latest` tag size, "
            "%d fell back to the median, and %d had no usable sized tag.\n\n"
            % (COST_LOW, COST_HIGH, len(rows), n_latest, n_median, n_none)
        )

        f.write("## Totals\n\n")
        f.write("| Metric | Value |\n|---|---:|\n")
        f.write(f"| Repositories | {len(rows):,} |\n")
        f.write(f"| Total lifetime pulls | {total_pulls:,} |\n")
        f.write(f"| Estimated bytes served | {_fmt_bytes(total_bytes)} |\n")
        f.write(f"| Estimated bytes (raw) | {total_bytes:,} |\n")
        f.write(f"| Cost @ ${COST_LOW:.2f}/GB | ${total_gb * COST_LOW:,.0f} |\n")
        f.write(f"| Cost @ ${COST_HIGH:.2f}/GB | ${total_gb * COST_HIGH:,.0f} |\n\n")

        f.write("## Concentration\n\n")
        f.write(f"- Top 10 repos account for **{share10:.1f}%** of estimated bytes "
                f"({_fmt_bytes(top10_bytes)}).\n")
        f.write(f"- Top 50 repos account for **{share50:.1f}%** of estimated bytes "
                f"({_fmt_bytes(top50_bytes)}).\n\n")

        f.write("## Top 50 by estimated bytes\n\n")
        f.write("| # | Repo | Pulls | Size (MB) | Src | Est. served | Cost @ $%.2f | Cost @ $%.2f |\n"
                % (COST_LOW, COST_HIGH))
        f.write("|---:|---|---:|---:|:--:|---:|---:|---:|\n")
        for i, r in enumerate(by_bytes[:50], 1):
            gb = r["est_bytes"] / GB
            f.write(
                f"| {i} | {r['name']} | {r['pull_count']:,} | "
                f"{r['full_size']/1e6:.1f} | {r['size_source'][:3]} | "
                f"{_fmt_bytes(r['est_bytes'])} | ${gb*COST_LOW:,.0f} | "
                f"${gb*COST_HIGH:,.0f} |\n"
            )
        f.write("\n")

        f.write("## Top 50 by pull count\n\n")
        f.write("| # | Repo | Pulls | Size (MB) | Est. served |\n")
        f.write("|---:|---|---:|---:|---:|\n")
        for i, r in enumerate(by_pulls[:50], 1):
            f.write(
                f"| {i} | {r['name']} | {r['pull_count']:,} | "
                f"{r['full_size']/1e6:.1f} | {_fmt_bytes(r['est_bytes'])} |\n"
            )
        f.write("\n")

        f.write("## Caveats\n\n")
        f.write(
            "- **`pull_count` counts manifest requests, not bytes.** Docker Hub "
            "increments the pull counter on manifest fetches. Multi-arch images "
            "are described by a manifest *list* plus a per-architecture manifest, "
            "so a single `docker pull` can register more than once, inflating the "
            "count. Conversely, layer caching means most pulls transfer far less "
            "than `full_size`: shared base layers (glibc, openssl, the distro "
            "rootfs) are downloaded once and reused, and re-pulls of an unchanged "
            "image move almost no bytes. So a pull is neither one image-download "
            "nor `full_size` bytes on the wire.\n"
        )
        f.write(
            "- **`latest` size is a point-in-time proxy.** We multiply *every* "
            "historical pull by *today's* `latest` (or median) compressed size. "
            "Image sizes drift substantially over a repo's life as base images, "
            "toolchains and contents change, and `latest` is not representative of "
            "the tag mix people actually pulled (alpine/slim variants, pinned "
            "versions, etc.).\n"
        )
        f.write(
            "- **This is an upper bound on wire bytes, probably 2-5x high.** "
            "Between manifest double-counting and layer caching, true egress is "
            "well below these figures. Treat the absolute dollar amounts as "
            "order-of-magnitude only -- the point is the *scale* of the subsidy "
            "and the *ranking* of which images dominate it, both of which are "
            "robust to the size proxy.\n"
        )
    sys.stderr.write(f"wrote {out}\n")


def print_headline(rows):
    total_pulls = sum(r["pull_count"] for r in rows)
    total_bytes = sum(r["est_bytes"] for r in rows)
    total_gb = total_bytes / GB
    by_bytes = sorted(rows, key=lambda x: x["est_bytes"], reverse=True)
    top10 = sum(r["est_bytes"] for r in by_bytes[:10])
    share10 = (top10 / total_bytes * 100) if total_bytes else 0

    print("\n" + "=" * 60)
    print("Docker Hub Official Images - estimated bandwidth subsidy")
    print("=" * 60)
    print(f"Repositories analysed : {len(rows):,}")
    print(f"Total lifetime pulls  : {total_pulls:,}")
    print(f"Estimated bytes served: {_fmt_bytes(total_bytes)}  ({total_bytes:,} B)")
    print(f"Egress cost @ ${COST_LOW:.2f}/GB : ${total_gb * COST_LOW:,.0f}")
    print(f"Egress cost @ ${COST_HIGH:.2f}/GB : ${total_gb * COST_HIGH:,.0f}")
    print(f"Top 10 repos          : {share10:.1f}% of estimated bytes")
    print(f"Biggest single repo   : {by_bytes[0]['name']} "
          f"({_fmt_bytes(by_bytes[0]['est_bytes'])})")
    print("=" * 60)
    print("NOTE: upper bound, likely 2-5x high. See summary.md CAVEATS.")


if __name__ == "__main__":
    main()
