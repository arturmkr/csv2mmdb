package main

import (
	"bytes"
	"net/netip"
	"os"
	"strings"
	"testing"

	"github.com/oschwald/maxminddb-golang/v2"
)

func TestISPExample(t *testing.T) {
	csv, err := os.ReadFile("examples/isp.csv")
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := convert(bytes.NewReader(csv), &out, "GeoIP2-ISP", 4, 1700000001); err != nil {
		t.Fatal(err)
	}
	db, err := maxminddb.OpenBytes(out.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if db.Metadata.DatabaseType != "GeoIP2-ISP" || db.Metadata.BuildEpoch != 1700000001 {
		t.Fatalf("unexpected metadata: %+v", db.Metadata)
	}
	for ip, want := range map[string]string{"203.0.113.42": "Lab ISP", "203.0.113.43": "Example ISP"} {
		var isp string
		if err := db.Lookup(netip.MustParseAddr(ip)).DecodePath(&isp, "isp"); err != nil || isp != want {
			t.Errorf("%s: got ISP %q, err %v; want %q", ip, isp, err, want)
		}
	}
	var record struct {
		ASN          uint32 `maxminddb:"autonomous_system_number"`
		Organization string `maxminddb:"organization"`
	}
	if err := db.Lookup(netip.MustParseAddr("203.0.113.42")).Decode(&record); err != nil {
		t.Fatal(err)
	}
	if record.ASN != 64513 || record.Organization != "Example, Inc." {
		t.Errorf("unexpected ISP record: %+v", record)
	}
	if db.Lookup(netip.MustParseAddr("192.0.2.1")).Found() {
		t.Error("unexpected match outside the CSV networks")
	}
}

func TestCityIPv6(t *testing.T) {
	const csv = "city,cidr,city_id,latitude,registered_country_code\nTest City,2001:db8::/64,12345,-12.5,GB\n"
	var out bytes.Buffer
	if err := convert(strings.NewReader(csv), &out, "GeoIP2-City", 6, 1700000001); err != nil {
		t.Fatal(err)
	}
	db, err := maxminddb.OpenBytes(out.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var record struct {
		City struct {
			ID    uint32            `maxminddb:"geoname_id"`
			Names map[string]string `maxminddb:"names"`
		} `maxminddb:"city"`
		Location struct {
			Latitude float64 `maxminddb:"latitude"`
		} `maxminddb:"location"`
		Registered struct {
			ISO string `maxminddb:"iso_code"`
		} `maxminddb:"registered_country"`
	}
	if err := db.Lookup(netip.MustParseAddr("2001:db8::abcd")).Decode(&record); err != nil {
		t.Fatal(err)
	}
	if record.City.Names["en"] != "Test City" || record.City.ID != 12345 ||
		record.Location.Latitude != -12.5 || record.Registered.ISO != "GB" {
		t.Errorf("unexpected City record: %+v", record)
	}
}

func TestInvalidCSVWritesNoDatabase(t *testing.T) {
	for _, tc := range []struct{ edition, csv, want string }{
		{"GeoIP2-ISP", "cidr,isp\n", "no data rows"},
		{"GeoIP2-ISP", "isp,organization\n", "cidr column"},
		{"GeoIP2-ISP", "cidr,isp,isp\n", "duplicate"},
		{"GeoIP2-City", "cidr,isp\n", "unknown column"},
		{"GeoIP2-ISP", "cidr,isp\n203.0.113.42/24,Test\n", "host bits"},
		{"GeoIP2-ISP", "cidr,isp\n2001:db8::42,Test\n", "--ip-version 6"},
		{"GeoIP2-ISP", "cidr,isp\n203.0.113.42,\n", "empty"},
		{"GeoIP2-ISP", "cidr,asn\n203.0.113.42,123\n192.0.2.1,-1\n", "row 3, asn"},
		{"GeoIP2-City", "cidr,latitude\n192.0.2.1,NaN\n", "row 2, latitude"},
	} {
		var out bytes.Buffer
		err := convert(strings.NewReader(tc.csv), &out, tc.edition, 4, 1)
		if err == nil || !strings.Contains(err.Error(), tc.want) || out.Len() != 0 {
			t.Errorf("CSV %q: got %v and %d output bytes; want %q and no output", tc.csv, err, out.Len(), tc.want)
		}
	}
}
