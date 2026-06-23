# Docker Hub Official Images: estimated bandwidth served

_Generated from the Docker Hub v2 API. 179 repositories in the `library/` namespace._

## Methodology

Every repository in Docker Hub's `library/` namespace (the curated **Official Images** programme) was enumerated through the public `/v2/repositories/library/` API, capturing each repo's lifetime `pull_count`. For each repo the compressed image size was taken from the tags endpoint: the `full_size` of the `latest` tag where it exists, otherwise the median `full_size` across the first page of tags. Estimated bytes served per repo is simply `pull_count x full_size`; these are summed across the namespace and priced at $0.02/GB (Cloudflare-ish bandwidth-alliance rate) and $0.09/GB (AWS list-price egress). Sizes are decimal GB/TB, matching how egress is metered. Of 179 repos, 127 used the `latest` tag size, 50 fell back to the median, and 2 had no usable sized tag.

## Totals

| Metric | Value |
|---|---:|
| Repositories | 179 |
| Total lifetime pulls | 165,038,573,818 |
| Estimated bytes served | 18989.82 PB |
| Estimated bytes (raw) | 18,989,816,898,205,963,662 |
| Cost @ $0.02/GB | $379,796,338 |
| Cost @ $0.09/GB | $1,709,083,521 |

## Concentration

- Top 10 repos account for **65.5%** of estimated bytes (12431.50 PB).
- Top 50 repos account for **95.9%** of estimated bytes (18217.29 PB).

## Top 50 by estimated bytes

| # | Repo | Pulls | Size (MB) | Src | Est. served | Cost @ $0.02 | Cost @ $0.09 |
|---:|---|---:|---:|:--:|---:|---:|---:|
| 1 | mongo | 4,784,689,318 | 941.2 | med | 4503.46 PB | $90,069,139 | $405,311,125 |
| 2 | mysql | 5,008,536,621 | 270.2 | lat | 1353.18 PB | $27,063,553 | $121,785,988 |
| 3 | sonarqube | 1,212,125,634 | 1077.0 | lat | 1305.42 PB | $26,108,440 | $117,487,980 |
| 4 | postgres | 10,856,245,227 | 115.5 | med | 1254.15 PB | $25,082,959 | $112,873,314 |
| 5 | openjdk | 2,610,392,877 | 420.0 | med | 1096.26 PB | $21,925,147 | $98,663,162 |
| 6 | elasticsearch | 961,659,808 | 721.6 | med | 693.94 PB | $13,878,724 | $62,454,256 |
| 7 | ruby | 1,553,455,325 | 428.7 | lat | 665.92 PB | $13,318,380 | $59,932,712 |
| 8 | node | 6,588,005,506 | 81.5 | med | 536.84 PB | $10,736,772 | $48,315,473 |
| 9 | mariadb | 3,118,328,157 | 163.9 | med | 511.22 PB | $10,224,389 | $46,009,750 |
| 10 | nextcloud | 1,015,133,961 | 503.5 | lat | 511.12 PB | $10,222,467 | $46,001,101 |
| 11 | docker | 3,507,659,323 | 142.1 | lat | 498.40 PB | $9,968,082 | $44,856,370 |
| 12 | rabbitmq | 3,822,019,142 | 116.1 | lat | 443.80 PB | $8,876,087 | $39,942,392 |
| 13 | memcached | 13,198,050,626 | 32.2 | lat | 425.04 PB | $8,500,817 | $38,253,677 |
| 14 | ubuntu | 9,925,592,356 | 41.6 | lat | 412.53 PB | $8,250,674 | $37,128,032 |
| 15 | redis | 10,859,889,823 | 37.6 | med | 408.61 PB | $8,172,164 | $36,774,738 |
| 16 | nginx | 13,102,789,049 | 26.0 | med | 340.45 PB | $6,808,973 | $30,640,377 |
| 17 | golang | 2,573,814,908 | 106.6 | med | 274.49 PB | $5,489,896 | $24,704,534 |
| 18 | httpd | 4,715,409,591 | 45.2 | lat | 213.30 PB | $4,266,091 | $19,197,411 |
| 19 | traefik | 3,501,988,894 | 52.9 | lat | 185.43 PB | $3,708,509 | $16,688,289 |
| 20 | solr | 353,794,246 | 468.2 | lat | 165.66 PB | $3,313,126 | $14,909,067 |
| 21 | python | 8,875,059,031 | 18.3 | med | 162.37 PB | $3,247,479 | $14,613,657 |
| 22 | maven | 752,497,932 | 174.6 | med | 131.39 PB | $2,627,812 | $11,825,155 |
| 23 | tomcat | 816,252,347 | 158.1 | lat | 129.05 PB | $2,581,096 | $11,614,934 |
| 24 | neo4j | 315,329,431 | 371.2 | lat | 117.05 PB | $2,341,094 | $10,534,921 |
| 25 | telegraf | 636,770,191 | 175.3 | lat | 111.64 PB | $2,232,749 | $10,047,371 |
| 26 | wordpress | 1,471,343,735 | 73.0 | med | 107.35 PB | $2,147,028 | $9,661,627 |
| 27 | logstash | 201,507,558 | 518.0 | med | 104.38 PB | $2,087,615 | $9,394,268 |
| 28 | gradle | 293,857,773 | 344.9 | med | 101.34 PB | $2,026,814 | $9,120,662 |
| 29 | kibana | 223,355,787 | 453.2 | med | 101.22 PB | $2,024,444 | $9,109,998 |
| 30 | ghost | 376,575,545 | 268.4 | lat | 101.06 PB | $2,021,146 | $9,095,159 |
| 31 | influxdb | 1,100,172,703 | 91.1 | med | 100.28 PB | $2,005,566 | $9,025,048 |
| 32 | perl | 253,061,101 | 394.8 | med | 99.92 PB | $1,998,398 | $8,992,789 |
| 33 | buildpack-deps | 255,160,679 | 378.9 | lat | 96.69 PB | $1,933,772 | $8,701,974 |
| 34 | centos | 1,183,214,797 | 75.8 | med | 89.67 PB | $1,793,404 | $8,070,317 |
| 35 | debian | 1,648,462,623 | 48.8 | med | 80.41 PB | $1,608,224 | $7,237,006 |
| 36 | couchbase | 89,738,187 | 890.1 | lat | 79.88 PB | $1,597,578 | $7,189,100 |
| 37 | rust | 127,463,357 | 594.1 | lat | 75.73 PB | $1,514,607 | $6,815,734 |
| 38 | percona | 232,083,793 | 321.5 | med | 74.62 PB | $1,492,490 | $6,716,204 |
| 39 | flink | 95,915,011 | 664.8 | lat | 63.77 PB | $1,275,318 | $5,738,931 |
| 40 | consul | 1,055,551,005 | 56.2 | med | 59.32 PB | $1,186,399 | $5,338,796 |
| 41 | php | 1,319,265,930 | 41.5 | med | 54.72 PB | $1,094,363 | $4,924,635 |
| 42 | amazonlinux | 967,458,840 | 54.6 | lat | 52.80 PB | $1,055,966 | $4,751,845 |
| 43 | alpine | 11,956,275,332 | 3.8 | lat | 45.99 PB | $919,770 | $4,138,966 |
| 44 | vault | 552,864,785 | 81.8 | med | 45.24 PB | $904,742 | $4,071,337 |
| 45 | kong | 355,245,282 | 122.8 | lat | 43.63 PB | $872,658 | $3,926,959 |
| 46 | cassandra | 256,869,265 | 169.1 | lat | 43.45 PB | $868,905 | $3,910,071 |
| 47 | zookeeper | 348,372,120 | 117.4 | lat | 40.91 PB | $818,295 | $3,682,327 |
| 48 | odoo | 50,843,057 | 710.6 | lat | 36.13 PB | $722,573 | $3,251,578 |
| 49 | registry | 1,752,548,240 | 20.1 | lat | 35.26 PB | $705,270 | $3,173,715 |
| 50 | sentry | 124,297,833 | 263.8 | lat | 32.79 PB | $655,831 | $2,951,241 |

## Top 50 by pull count

| # | Repo | Pulls | Size (MB) | Est. served |
|---:|---|---:|---:|---:|
| 1 | memcached | 13,198,050,626 | 32.2 | 425.04 PB |
| 2 | nginx | 13,102,789,049 | 26.0 | 340.45 PB |
| 3 | busybox | 12,631,628,382 | 2.2 | 28.12 PB |
| 4 | alpine | 11,956,275,332 | 3.8 | 45.99 PB |
| 5 | redis | 10,859,889,823 | 37.6 | 408.61 PB |
| 6 | postgres | 10,856,245,227 | 115.5 | 1254.15 PB |
| 7 | ubuntu | 9,925,592,356 | 41.6 | 412.53 PB |
| 8 | python | 8,875,059,031 | 18.3 | 162.37 PB |
| 9 | node | 6,588,005,506 | 81.5 | 536.84 PB |
| 10 | mysql | 5,008,536,621 | 270.2 | 1353.18 PB |
| 11 | mongo | 4,784,689,318 | 941.2 | 4503.46 PB |
| 12 | httpd | 4,715,409,591 | 45.2 | 213.30 PB |
| 13 | rabbitmq | 3,822,019,142 | 116.1 | 443.80 PB |
| 14 | docker | 3,507,659,323 | 142.1 | 498.40 PB |
| 15 | traefik | 3,501,988,894 | 52.9 | 185.43 PB |
| 16 | hello-world | 3,305,033,955 | 0.0 | 7.98 TB |
| 17 | mariadb | 3,118,328,157 | 163.9 | 511.22 PB |
| 18 | openjdk | 2,610,392,877 | 420.0 | 1096.26 PB |
| 19 | golang | 2,573,814,908 | 106.6 | 274.49 PB |
| 20 | registry | 1,752,548,240 | 20.1 | 35.26 PB |
| 21 | debian | 1,648,462,623 | 48.8 | 80.41 PB |
| 22 | ruby | 1,553,455,325 | 428.7 | 665.92 PB |
| 23 | wordpress | 1,471,343,735 | 73.0 | 107.35 PB |
| 24 | php | 1,319,265,930 | 41.5 | 54.72 PB |
| 25 | sonarqube | 1,212,125,634 | 1077.0 | 1305.42 PB |
| 26 | centos | 1,183,214,797 | 75.8 | 89.67 PB |
| 27 | haproxy | 1,121,201,298 | 17.9 | 20.05 PB |
| 28 | influxdb | 1,100,172,703 | 91.1 | 100.28 PB |
| 29 | consul | 1,055,551,005 | 56.2 | 59.32 PB |
| 30 | nextcloud | 1,015,133,961 | 503.5 | 511.12 PB |
| 31 | amazonlinux | 967,458,840 | 54.6 | 52.80 PB |
| 32 | elasticsearch | 961,659,808 | 721.6 | 693.94 PB |
| 33 | tomcat | 816,252,347 | 158.1 | 129.05 PB |
| 34 | maven | 752,497,932 | 174.6 | 131.39 PB |
| 35 | caddy | 696,571,770 | 23.9 | 16.65 PB |
| 36 | eclipse-mosquitto | 668,890,630 | 10.0 | 6.68 PB |
| 37 | telegraf | 636,770,191 | 175.3 | 111.64 PB |
| 38 | bash | 577,879,951 | 4.9 | 2.81 PB |
| 39 | vault | 552,864,785 | 81.8 | 45.24 PB |
| 40 | adminer | 398,779,124 | 46.7 | 18.62 PB |
| 41 | ghost | 376,575,545 | 268.4 | 101.06 PB |
| 42 | kong | 355,245,282 | 122.8 | 43.63 PB |
| 43 | solr | 353,794,246 | 468.2 | 165.66 PB |
| 44 | zookeeper | 348,372,120 | 117.4 | 40.91 PB |
| 45 | neo4j | 315,329,431 | 371.2 | 117.05 PB |
| 46 | gradle | 293,857,773 | 344.9 | 101.34 PB |
| 47 | eclipse-temurin | 293,606,496 | 73.7 | 21.65 PB |
| 48 | mongo-express | 273,983,647 | 58.9 | 16.15 PB |
| 49 | cassandra | 256,869,265 | 169.1 | 43.45 PB |
| 50 | buildpack-deps | 255,160,679 | 378.9 | 96.69 PB |

## Caveats

- **`pull_count` counts manifest requests, not bytes.** Docker Hub increments the pull counter on manifest fetches. Multi-arch images are described by a manifest *list* plus a per-architecture manifest, so a single `docker pull` can register more than once, inflating the count. Conversely, layer caching means most pulls transfer far less than `full_size`: shared base layers (glibc, openssl, the distro rootfs) are downloaded once and reused, and re-pulls of an unchanged image move almost no bytes. So a pull is neither one image-download nor `full_size` bytes on the wire.
- **`latest` size is a point-in-time proxy.** We multiply *every* historical pull by *today's* `latest` (or median) compressed size. Image sizes drift substantially over a repo's life as base images, toolchains and contents change, and `latest` is not representative of the tag mix people actually pulled (alpine/slim variants, pinned versions, etc.).
- **This is an upper bound on wire bytes, probably 2-5x high.** Between manifest double-counting and layer caching, true egress is well below these figures. Treat the absolute dollar amounts as order-of-magnitude only -- the point is the *scale* of the subsidy and the *ranking* of which images dominate it, both of which are robust to the size proxy.
