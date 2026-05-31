package geo

import (
	"strings"

	"github.com/example/cc-panel/internal/ipset"
)

func (s *Service) MatchesDefaultWhitelistIP(ip string) (bool, error) {
	ip = ipset.NormalizeIP(ip)
	info, err := s.SearchIP(ip)
	if err != nil {
		return false, err
	}
	return MatchesDefaultWhitelistRegion(info), nil
}

func MatchesDefaultWhitelistRegion(info RegionInfo) bool {
	for _, candidate := range defaultWhitelistCountryNames(info) {
		for _, allowed := range defaultWhitelistCountries {
			if candidate == allowed {
				return true
			}
		}
	}
	return false
}

func defaultWhitelistCountryNames(info RegionInfo) []string {
	country := strings.TrimSpace(info.Country)
	province := strings.TrimSpace(info.Province)
	names := []string{country}
	switch country {
	case "中国", "China":
		switch {
		case strings.Contains(province, "香港"):
			names = append(names, "中国香港", "香港")
		case strings.Contains(province, "台湾"):
			names = append(names, "中国台湾", "台湾")
		case strings.Contains(province, "澳门"):
			names = append(names, "中国澳门", "澳门")
		default:
			names = append(names, "中国内地", "中国")
		}
	case "香港", "Hong Kong":
		names = append(names, "中国香港")
	case "台湾", "Taiwan":
		names = append(names, "中国台湾")
	}
	return names
}
