#!/usr/bin/env ruby
# Merge corpus sources into seed.txt, deduped on repo URL.
# Prefers purls (more specific) over bare repo URLs when both point at the same repo.
# examples.txt is excluded — bare names need resolving first.

dir = __dir__

def norm(url)
  return nil if url.nil? || url.empty?
  url.strip.downcase.sub(%r{\.git$}, "").sub(%r{/$}, "")
end

by_repo = {} # norm(repo_url) => preferred identifier (purl or url)

# top-deps: purl<TAB>repo_url — purl wins
File.foreach(File.join(dir, "top-deps.txt")) do |line|
  next if line.start_with?("#") || line.strip.empty?
  purl, repo = line.chomp.split("\t", 2)
  key = norm(repo) || purl
  by_repo[key] ||= purl
end

# knowledge: bare repo URLs — add only if repo not already covered
File.foreach(File.join(dir, "knowledge.txt")) do |line|
  next if line.start_with?("#") || line.strip.empty?
  url = line.strip
  key = norm(url)
  by_repo[key] ||= url
end

out = File.open(File.join(dir, "seed.txt"), "w")
out.puts "# Merged corpus seed: top-deps.txt + knowledge.txt, deduped on repo URL."
out.puts "# Generated #{Time.now.utc.strftime("%Y-%m-%d")}. #{by_repo.size} entries."
out.puts "# examples.txt excluded (unresolved bare names)."
out.puts
by_repo.values.sort.each { |id| out.puts id }
out.close

warn "wrote #{by_repo.size} entries to #{File.join(dir, "seed.txt")}"
