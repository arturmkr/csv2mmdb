# csv2mmdb

Make a small, synthetic MaxMind DB for a test. Feed CSV to stdin and get a
binary `.mmdb` on stdout. Uses the [MaxMind Go writer](https://github.com/maxmind/mmdbwriter);
it does not download or include any commercial GeoIP data.

## Make a fixture

Build a local image (no Go installation needed):

```sh
docker build -t csv2mmdb:local .
docker run --rm -i --network=none csv2mmdb:local \
  --type GeoIP2-ISP --build-epoch 1700000001 < examples/isp.csv > test-isp.mmdb
```

Or replace `csv2mmdb:local` with `<dockerhub-username>/csv2mmdb:v1.0.0`
once that tag is published. Use `-i`, **not** `-t`: the output is binary.
The CSV and the generated file stay on your machine.

For example, to make `203.0.113.42` return a different ISP in a hot-reload test,
change the CSV cell rather than modifying code:

```sh
printf 'cidr,isp\n203.0.113.42,ISP-v2\n' |
  docker run --rm -i --network=none csv2mmdb:local \
    --type GeoIP2-ISP --build-epoch 1700000002 > test-isp-v2.mmdb
```

For City, use `--type GeoIP2-City` and [examples/city.csv](examples/city.csv).
Check the resulting `test-*.mmdb` into the consuming project's test resources;
its tests need neither this generator nor Docker Hub to read them.

## CSV format

Start with a header containing `cidr` and one or more columns from the chosen
edition. Column order does not matter. Blank data cells are omitted. Use standard
CSV quoting for values containing commas. Unknown headers and malformed rows
produce an error on stderr and a non-zero exit status.

| `--type` | Supported data columns |
| --- | --- |
| `GeoIP2-ISP` | `isp`, `organization`, `asn`, `as_org` |
| `GeoIP2-City` | `registered_country_code`, `country_code`, `country`, `country_id`, `city`, `city_id`, `latitude`, `longitude`, `continent_code`, `continent` |

- `cidr` accepts a bare IP or a network prefix with its host bits cleared.
  IPv4 is the default; use `--ip-version 6` for IPv6 or mixed input.
  Documentation ranges and private networks are allowed.
- Records are inserted in CSV order: put a broad prefix before a more
  specific override, as in [examples/isp.csv](examples/isp.csv).
- Names are stored in English. ASNs and GeoNames IDs are unsigned 32-bit
  integers; coordinates are decimal degrees.
- Set a positive `--build-epoch` (Unix seconds) and pin the tool version
  for byte-for-byte reproducibility. By default the writer uses the current time.

## Local development

With Go 1.26 or newer:

```sh
go test ./...
go run . --type GeoIP2-City < examples/city.csv > test-city.mmdb
```
