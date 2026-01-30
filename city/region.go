package city

var Regions = func() []Name {
	return []Name{
		{
			Short: []string{"东北"},
			CN:    "东北大区",
			EN:    "Northeast",
			Code:  "Northeast",
		},
		{
			Short: []string{"华北"},
			CN:    "华北大区",
			EN:    "NorthChina",
			Code:  "NorthChina",
		},
		{
			Short: []string{"华中"},
			CN:    "华中大区",
			EN:    "CentralChina",
			Code:  "CentralChina",
		},
		{
			Short: []string{"华东"},
			CN:    "华东大区",
			EN:    "EastChina",
			Code:  "EastChina",
		},
		{
			Short: []string{"华南"},
			CN:    "华南大区",
			EN:    "SouthChina",
			Code:  "SouthChina",
		},
		{
			Short: []string{"西南"},
			CN:    "西南大区",
			EN:    "Southwest",
			Code:  "Southwest",
		},
		{
			Short: []string{"西北"},
			CN:    "西北大区",
			EN:    "Northwest",
			Code:  "Northwest",
		},
	}
}

var ProvinceRegionMap = func() map[string]string {
	return map[string]string{
		"BJ": "NorthChina",
		"TJ": "NorthChina",
		"HE": "NorthChina",
		"SX": "NorthChina",
		"NM": "NorthChina",
		"LN": "Northeast",
		"JL": "Northeast",
		"HL": "Northeast",
		"SH": "EastChina",
		"JS": "EastChina",
		"ZJ": "EastChina",
		"AH": "EastChina",
		"FJ": "EastChina",
		"JX": "EastChina",
		"SD": "EastChina",
		"HA": "CentralChina",
		"HB": "CentralChina",
		"HN": "CentralChina",
		"GD": "SouthChina",
		"GX": "SouthChina",
		"HI": "SouthChina",
		"CQ": "Southwest",
		"SC": "Southwest",
		"GZ": "Southwest",
		"YN": "Southwest",
		"XZ": "Southwest",
		"SN": "Northwest",
		"GS": "Northwest",
		"QH": "Northwest",
		"NX": "Northwest",
		"XJ": "Northwest",
	}
}

// Region 为中国大区信息
var Region = genFunc(Regions())

// RegionOfProvince 返回一个省份的大区
func RegionOfProvince(name string) Name {
	v, ok := ProvinceRegionMap()[name]
	if !ok {
		return EmptyName
	}
	return Region(v)
}

func ProvincesInRegion(name string) []Name {
	region := Region(name)
	var data []Name
	for p, r := range ProvinceRegionMap() {
		if r == region.Code {
			data = append(data, Province(p))
		}
	}
	return data
}
