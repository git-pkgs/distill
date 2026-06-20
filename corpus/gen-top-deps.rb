#!/usr/bin/env ruby
# Fetch the top-N most-depended-on packages per registry from packages.ecosyste.ms
# and write purls + repo URLs to corpus/top-deps.txt.

require "json"
require "net/http"
require "uri"

REGISTRIES = %w[
  npmjs.org
  pypi.org
  rubygems.org
  crates.io
  proxy.golang.org
  repo1.maven.org
  packagist.org
  nuget.org
  hex.pm
  pub.dev
]

PER_REGISTRY = (ARGV[0] || 50).to_i
BASE = "https://packages.ecosyste.ms/api/v1/registries"

def fetch(registry, n)
  # dependent_repos_count, not dependent_packages_count: the latter is trivially
  # gamed by publishing N packages that all depend on each other (rubygems
  # top-50 by package count is ~40 entries of superjagger/* spam).
  uri = URI("#{BASE}/#{registry}/packages?sort=dependent_repos_count&order=desc&per_page=#{n}")
  res = Net::HTTP.get_response(uri)
  raise "#{registry}: #{res.code}" unless res.is_a?(Net::HTTPSuccess)
  JSON.parse(res.body)
end

out = File.open(File.join(__dir__, "top-deps.txt"), "w")
out.puts "# Top-#{PER_REGISTRY} packages per registry by dependent_repos_count from packages.ecosyste.ms."
out.puts "# Generated #{Time.now.utc.strftime("%Y-%m-%d")}. Format: purl<TAB>repo_url"
out.puts

REGISTRIES.each do |reg|
  warn "fetching #{reg}..."
  out.puts "# #{reg}"
  # Over-fetch then keep the first PER_REGISTRY that have a repo URL, since
  # distill needs source to clone (nuget in particular has many repo-less entries).
  kept = 0
  fetch(reg, PER_REGISTRY * 2).each do |pkg|
    purl = pkg["purl"]
    repo = pkg["repository_url"]
    next if purl.nil? || purl.empty? || repo.nil? || repo.empty?
    out.puts "#{purl}\t#{repo}"
    kept += 1
    break if kept >= PER_REGISTRY
  end
  out.puts
rescue => e
  warn "skip #{reg}: #{e.message}"
end

out.close
warn "wrote #{File.join(__dir__, "top-deps.txt")}"
