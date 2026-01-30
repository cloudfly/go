package city

var Provinces = func() []Name {
	return []Name{
		{Code: "SH", CN: "上海", EN: "Shanghai", Short: []string{"SH"}},
		{Code: "QH", CN: "青海", EN: "Qinghai", Short: []string{"QH"}},
		{Code: "MO", CN: "澳门", EN: "Macao", Short: []string{"MO"}},
		{Code: "HB", CN: "湖北", EN: "Hubei", Short: []string{"HB"}},
		{Code: "AH", CN: "安徽", EN: "Anhui", Short: []string{"AH"}},
		{Code: "SX", CN: "山西", EN: "Shanxi", Short: []string{"SX"}},
		{Code: "JX", CN: "江西", EN: "Jiangxi", Short: []string{"JX"}},
		{Code: "BJ", CN: "北京", EN: "Beijing", Short: []string{"BJ"}},
		{Code: "HL", CN: "黑龙江", EN: "Heilongjiang", Short: []string{"HL"}},
		{Code: "JS", CN: "江苏", EN: "Jiangsu", Short: []string{"JS"}},
		{Code: "NM", CN: "内蒙古", EN: "Inner Mongolia Autonomous Region", Short: []string{"NM"}},
		{Code: "HI", CN: "海南", EN: "Hainan", Short: []string{"HI"}},
		{Code: "JL", CN: "吉林", EN: "Jilin", Short: []string{"JL"}},
		{Code: "TW", CN: "台湾", EN: "Taiwan", Short: []string{"TW"}},
		{Code: "FJ", CN: "福建", EN: "Fujian", Short: []string{"FJ"}},
		{Code: "XJ", CN: "新疆", EN: "Xinjiang", Short: []string{"XJ"}},
		{Code: "HN", CN: "湖南", EN: "Hunan", Short: []string{"HN", "HUN"}},
		{Code: "HE", CN: "河北", EN: "Hebei", Short: []string{"HE"}},
		{Code: "CQ", CN: "重庆", EN: "Chongqing", Short: []string{"CQ"}},
		{Code: "GD", CN: "广东", EN: "Guangdong", Short: []string{"GD"}},
		{Code: "HK", CN: "香港", EN: "Hong Kong", Short: []string{"HK"}},
		{Code: "SD", CN: "山东", EN: "Shandong", Short: []string{"SD"}},
		{Code: "SN", CN: "陕西", EN: "Shaanxi", Short: []string{"SN", "sshx"}},
		{Code: "ZJ", CN: "浙江", EN: "Zhejiang", Short: []string{"ZJ"}},
		{Code: "SC", CN: "四川", EN: "Sichuan", Short: []string{"SC"}},
		{Code: "GX", CN: "广西", EN: "Guangxi", Short: []string{"GX"}},
		{Code: "TJ", CN: "天津", EN: "Tianjin", Short: []string{"TJ"}},
		{Code: "HA", CN: "河南", EN: "Henan", Short: []string{"HA"}},
		{Code: "LN", CN: "辽宁", EN: "Liaoning", Short: []string{"LN"}},
		{Code: "GZ", CN: "贵州", EN: "Guizhou", Short: []string{"GZ"}},
		{Code: "XZ", CN: "西藏", EN: "Tibet", Short: []string{"XZ"}},
		{Code: "NX", CN: "宁夏", EN: "Ningxia Hui Autonomous Region", Short: []string{"NX"}},
		{Code: "YN", CN: "云南", EN: "Yunnan", Short: []string{"YN"}},
		{Code: "CN", CN: "中国", EN: "China", Short: []string{"CN"}},
		{Code: "GS", CN: "甘肃", EN: "Gansu", Short: []string{"GS"}},
	}
}

var Province = genFunc(Provinces())
