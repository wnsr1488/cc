package geo

import "testing"

func TestMatchesDefaultWhitelistRegionHongKong(t *testing.T) {
	info := RegionInfo{
		IP:       "43.198.186.11",
		Country:  "中国",
		Province: "香港特别行政区",
	}
	if !MatchesDefaultWhitelistRegion(info) {
		t.Fatalf("expected Hong Kong IP to match default whitelist countries")
	}
}

func TestMatchesDefaultWhitelistRegionMainland(t *testing.T) {
	info := RegionInfo{
		IP:       "183.25.168.172",
		Country:  "中国",
		Province: "广东省",
	}
	if !MatchesDefaultWhitelistRegion(info) {
		t.Fatalf("expected mainland China IP to match default whitelist countries")
	}
}

func TestMatchesDefaultWhitelistRegionForeign(t *testing.T) {
	info := RegionInfo{
		IP:      "103.4.9.155",
		Country: "日本",
	}
	if MatchesDefaultWhitelistRegion(info) {
		t.Fatalf("expected foreign IP not to match default whitelist countries")
	}
}
