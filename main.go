package main

import (
	"encoding/csv"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"net"
	"os"
	"strconv"
	"strings"

	"github.com/maxmind/mmdbwriter"
	"github.com/maxmind/mmdbwriter/mmdbtype"
)

func main() {
	edition := flag.String("type", "", "required: GeoIP2-ISP or GeoIP2-City")
	ipVersion := flag.Int("ip-version", 4, "4 for IPv4 only, 6 for IPv4 and IPv6")
	epoch := flag.Int64("build-epoch", 0, "build time (Unix seconds); 0 uses the current time")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: csv2mmdb --type EDITION [options] < input.csv > test.mmdb")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 0 {
		flag.Usage()
		os.Exit(2)
	}
	if err := convert(os.Stdin, os.Stdout, *edition, *ipVersion, *epoch); err != nil {
		fmt.Fprintln(os.Stderr, "csv2mmdb:", err)
		os.Exit(1)
	}
}

// The CSV uses flat names; the reader SDK expects nested maps and specific MMDB types.
var schemas = map[string]map[string]string{
	"GeoIP2-ISP": {
		"isp": "isp", "organization": "organization",
		"asn": "autonomous_system_number", "as_org": "autonomous_system_organization",
	},
	"GeoIP2-City": {
		"registered_country_code": "registered_country.iso_code",
		"country_code":            "country.iso_code",
		"country":                 "country.names.en",
		"country_id":              "country.geoname_id",
		"city":                    "city.names.en",
		"city_id":                 "city.geoname_id",
		"latitude":                "location.latitude",
		"longitude":               "location.longitude",
		"continent_code":          "continent.code",
		"continent":               "continent.names.en",
	},
}

func convert(in io.Reader, out io.Writer, edition string, ipVersion int, epoch int64) error {
	schema, ok := schemas[edition]
	if !ok {
		return fmt.Errorf("unsupported --type %q: choose GeoIP2-ISP or GeoIP2-City", edition)
	}
	if (ipVersion != 4 && ipVersion != 6) || epoch < 0 {
		return errors.New("--ip-version must be 4 or 6 and --build-epoch must be non-negative")
	}
	opts := mmdbwriter.Options{
		DatabaseType: edition, BuildEpoch: epoch, IPVersion: ipVersion,
		RecordSize: 24, IncludeReservedNetworks: true,
	}
	r := csv.NewReader(in)
	header, err := r.Read()
	if err != nil {
		return fmt.Errorf("reading CSV header: %w", err)
	}
	cidr := -1
	seen := make(map[string]bool)
	for i, field := range header {
		name := strings.TrimSpace(strings.TrimPrefix(field, "\uFEFF"))
		header[i] = name
		if seen[name] {
			return fmt.Errorf("duplicate CSV column %q", name)
		}
		seen[name] = true
		if name == "cidr" {
			cidr = i
			continue
		}
		path, ok := schema[name]
		if !ok {
			return fmt.Errorf("unknown column %q for %s", name, edition)
		}
		if strings.HasSuffix(path, ".names.en") {
			opts.Languages = []string{"en"}
		}
	}
	if cidr == -1 || len(header) < 2 {
		return errors.New("expected a cidr column and at least one data column")
	}
	tree, err := mmdbwriter.New(opts)
	if err != nil {
		return err
	}
	for row := 2; ; row++ {
		fields, err := r.Read()
		if err == io.EOF {
			if row == 2 {
				return errors.New("CSV has a header but no data rows")
			}
			break
		}
		if err != nil {
			return err
		}
		network, err := parseNetwork(strings.TrimSpace(fields[cidr]), ipVersion)
		if err != nil {
			return fmt.Errorf("row %d, cidr: %w", row, err)
		}
		record := mmdbtype.Map{}
		for i, field := range fields {
			value := strings.TrimSpace(field)
			if i == cidr || value == "" {
				continue
			}
			if err := setField(record, schema[header[i]], value); err != nil {
				return fmt.Errorf("row %d, %s: %w", row, header[i], err)
			}
		}
		if len(record) == 0 {
			return fmt.Errorf("row %d: all data fields are empty", row)
		}
		if err := tree.Insert(network, record); err != nil {
			return fmt.Errorf("row %d: %w", row, err)
		}
	}
	_, err = tree.WriteTo(out)
	return err
}

func parseNetwork(s string, ipVersion int) (*net.IPNet, error) {
	if !strings.Contains(s, "/") {
		if strings.Contains(s, ":") {
			s += "/128"
		} else {
			s += "/32"
		}
	}
	ip, network, err := net.ParseCIDR(s)
	if err != nil {
		return nil, err
	}
	if !ip.Equal(network.IP) {
		return nil, fmt.Errorf("%s has host bits set; use %s", s, network)
	}
	if _, bits := network.Mask.Size(); bits == 128 && ipVersion == 4 {
		return nil, errors.New("IPv6 input requires --ip-version 6")
	}
	return network, nil
}

func setField(record mmdbtype.Map, path, s string) error {
	var value mmdbtype.DataType = mmdbtype.String(s)
	switch path {
	case "autonomous_system_number", "country.geoname_id", "city.geoname_id":
		n, err := strconv.ParseUint(s, 10, 32)
		if err != nil {
			return err
		}
		value = mmdbtype.Uint32(n)
	case "location.latitude", "location.longitude":
		limit := 180.0
		if path == "location.latitude" {
			limit = 90
		}
		n, err := strconv.ParseFloat(s, 64)
		if err != nil || math.IsNaN(n) || math.Abs(n) > limit {
			return fmt.Errorf("expected a coordinate between %g and %g, got %q", -limit, limit, s)
		}
		value = mmdbtype.Float64(n)
	}
	keys := strings.Split(path, ".")
	for _, key := range keys[:len(keys)-1] {
		k := mmdbtype.String(key)
		if record[k] == nil {
			record[k] = mmdbtype.Map{}
		}
		record = record[k].(mmdbtype.Map)
	}
	record[mmdbtype.String(keys[len(keys)-1])] = value
	return nil
}
