#!/usr/bin/env ruby
# Generate vocab.txt (used in the classify prompt) and terms.txt (used for
# validation) from oss-taxonomy's combined-taxonomy.json. vocab.txt includes
# aliases and a one-line description so the teacher maps e.g. "ORM" to
# function:data-mapping instead of reporting it as a gap.

require "json"

src = ARGV[0] || File.expand_path("~/code/ecosystems/oss-taxonomy/combined-taxonomy.json")
abort "not found: #{src}" unless File.exist?(src)
tax = JSON.parse(File.read(src))

facets = tax.keys.select { |k| tax[k].is_a?(Array) }.sort
terms = []
vocab = []
facets.each do |facet|
  tax[facet].sort_by { |t| t["name"] }.each do |t|
    key = "#{facet}:#{t["name"]}"
    terms << key
    line = key
    if (al = t["aliases"]) && !al.empty?
      line += " (aka: #{al.join(", ")})"
    end
    desc = (t["description"] || "").split(".").first
    line += " — #{desc.strip}" unless desc.to_s.empty?
    vocab << line
  end
end

dir = __dir__
File.write(File.join(dir, "terms.txt"), terms.join("\n") + "\n")
File.write(File.join(dir, "vocab.txt"), vocab.join("\n") + "\n")
warn "wrote #{terms.size} terms to terms.txt and vocab.txt"
