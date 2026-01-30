package city

var Isps = func() []Name {
	return []Name{
		{
			Code:  "Founder",
			CN:    "方正宽带",
			EN:    "fzkd",
			Short: []string{"fzkd"},
		},
		{
			Code:  "CMCC",
			CN:    "移动",
			EN:    "yd",
			Short: []string{"yd"},
		},
		{
			Code:  "GWBN",
			CN:    "长宽",
			EN:    "ck",
			Short: []string{"ck"},
		},
		{
			Code:  "CTT",
			CN:    "铁通",
			EN:    "tt",
			Short: []string{"tt"},
		},
		{
			Code:  "PCCW",
			CN:    "电讯盈科",
			EN:    "dxyk",
			Short: []string{"dxyk"},
		},
		{
			Code:  "EDU",
			CN:    "教育网",
			EN:    "jyw",
			Short: []string{"jyw"},
		},
		{
			Code:  "Wasu",
			CN:    "华数",
			EN:    "hs",
			Short: []string{"hs"},
		},
		{
			Code:  "Youchi",
			CN:    "游驰",
			EN:    "yc",
			Short: []string{"yc"},
		},
		{
			Code:  "Wexchange",
			CN:    "驰联",
			EN:    "cl",
			Short: []string{"cl"},
		},
		{
			Code:  "BGP",
			CN:    "BGP",
			EN:    "BGP",
			Short: []string{"BGP"},
		},
		{
			Code:  "BTVN",
			CN:    "广电",
			EN:    "gd",
			Short: []string{"gd"},
		},
		{
			Code:  "Watone",
			CN:    "华通云",
			EN:    "hty",
			Short: []string{"hty"},
		},
		{
			Code:  "Drpeng",
			CN:    "鹏博士",
			EN:    "pbs",
			Short: []string{"pbs"},
		},
		{
			Code:  "Cnean",
			CN:    "亿安天下",
			EN:    "yatx",
			Short: []string{"yatx"},
		},
		{
			Code:  "Topway",
			CN:    "天威视讯",
			EN:    "twsx",
			Short: []string{"twsx"},
		},
		{
			Code:  "CTCC",
			CN:    "电信",
			EN:    "dx",
			Short: []string{"dx"},
		},
		{
			Code:  "CUCC",
			CN:    "联通",
			EN:    "lt",
			Short: []string{"lt"},
		},
		{
			Code:  "CMCC_CTCC_CUCC",
			CN:    "移动_电信_联通",
			EN:    "yd_dx_lt",
			Short: []string{"yd_dx_lt"},
		},
		{
			Code:  "Ifeixiang",
			CN:    "飞享",
			EN:    "fx",
			Short: []string{"fx"},
		},
		{
			Code:  "BGCTV",
			CN:    "歌华有线",
			EN:    "bjgh",
			Short: []string{"bjgh"},
		},
		{
			Code:  "ZJSM",
			CN:    "宽频",
			EN:    "kp",
			Short: []string{"kp"},
		},
		{
			Code:  "OTHER",
			CN:    "其它",
			EN:    "other",
			Short: []string{"other"},
		},
	}
}

// Isp 为国内运营商信息
var Isp = genFunc(Isps())
