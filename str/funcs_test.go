package str

import (
	"encoding/json"
	"testing"
)

func TestMatchDst(t *testing.T) {
	pattern := "*[2??]*[2??]*"

	log := "[d88bba12217d98e1343fe8a921d3079a] [2022-09-15T15:08:03+08:00] [http] [100.64.2.33:5794] [36.136.125.149:3139:80] [xwt-0802.nanxinef.top] [111.62.37.99:32630] [200] [172.20.96.223:8083] [200] [res-22002129118022250023|res-25115000229002001122] [GET / HTTP/1.1] [103] [287] [test-001.tuzihemao.xyz] [Go-http-client/1.1] [-] [0.001] [0.000] [0.000] [0.000] [43.110] [76962240] [1] [-] [-] [-]"

	var dst []string
	dst, ok := MatchDst(pattern, log, dst)
	if !ok {
		t.Log("should be ok")
		t.Fail()
	}
	content, _ := json.MarshalIndent(dst, "", "  ")
	t.Log(string(content))

}

func TestReplaceVars(t *testing.T) {
	vars := map[string]any{
		"name": "chenyunfei",
	}

	if s := ReplaceVars("$name", vars); s != "chenyunfei" {
		t.Log(s, "!= chenyunfei")
		t.Fail()
	}

	if s := ReplaceVars("hello $name", vars); s != "hello chenyunfei" {
		t.Log(s, "!= chenyunfei")
		t.Fail()
	}
}

func TestEncodeId(t *testing.T) {
	for _, item := range []uint64{1, 20, 21, 22, 60, 100, 101, 102, 232, 2732432, 923741341, 91827431983247} {
		id := EncodeId("p-", item)
		t.Log(id)
		n, err := DecodeId("p-", id)
		if err != nil {
			t.Log(err)
			t.Fail()
		}
		if n != item {
			t.Log(n, "!= ", item)
			t.Fail()
		}
	}
}
