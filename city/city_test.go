package city

import "testing"

func TestCityIspFromAlias(t *testing.T) {

	cases := [][2]string{
		{"zjwzct", "温州电信"},
		{"shcm", "上海移动"},
		{"bdcdn-hbxtcu03", "仙桃联通"},
		{"ahhfcu04", "合肥联通"},
		{"admin01.zzct03", "郑州电信"},
		{"gdgzct06", "广州电信"},
		{"zjwzct03", "温州电信"},
		{"cache08.fjfzct01", "福州电信"},
		{"tycm08", "太原移动"},
		{"zqcm02", "肇庆移动"},
		{"zjhzcu01", "杭州联通"},
		{"bdct", "保定电信"},
		{"admin01.chdcu03", "常德联通"},
		{"admin01.gxcu", "广西联通"},
		{"admin01.huncu", "湖南联通"},
		{"admin02.gdcu", "广东联通"},
		{"admin02.xzcu", "西藏联通"},
	}

	for _, c := range cases {
		if value := CityIspFromAlias(c[0]); value != c[1] {
			t.Errorf("%s => %s, it should be %s", c[0], value, c[1])
		}
	}
}
