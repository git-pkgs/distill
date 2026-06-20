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
  fetch(reg, PER_REGISTRY).each do |pkg|
    purl = pkg["purl"]
    repo = pkg["repository_url"]
    next if purl.nil? || purl.empty?
    out.puts [purl, repo].compact.join("\t")
  end
  out.puts
rescue => e
  warn "skip #{reg}: #{e.message}"
end

out.close
warn "wrote #{File.join(__dir__, "top-deps.txt")}"
