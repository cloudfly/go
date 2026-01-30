package filterpolicy

import (
	"testing"
	"time"

	"github.com/cloudfly/go/test"
)

func TestPass(t *testing.T) {
	assert, require := test.Assert(t), test.Require(t)

	p, err := Parse("0s,*/10s")
	require.NoError(err)
	start := time.Date(2020, 8, 28, 15, 2, 47, 0, time.Local)

	now := time.Now().Truncate(time.Second)
	for i := 0; i < 80; i++ {
		current := now.Add(time.Second * time.Duration(i))
		if int(current.Sub(start).Seconds())%10 == 0 {
			assert.True(p.Pass(start, current))
		} else {
			assert.False(p.Pass(start, current))
		}
	}
	p, err = Parse("*/1h,1h10m,1h20m,1h30m")
	assert.False(p.Pass(start, start))
	require.NoError(err)
	for i := 0; i < 7200; i++ {
		current := start.Add(time.Second * time.Duration(i))
		if i != 0 && (i%3600 == 0 || i == 4200 || i == 4800 || i == 5400) {
			assert.True(p.Pass(start, current))
		} else {
			assert.False(p.Pass(start, current))
		}
	}

	p, err = Parse("11s-3m1s/10s")
	require.NoError(err)
	for i := 0; i < 200; i++ {
		current := start.Add(time.Second * time.Duration(i))
		if i%10 == 0 && i >= 11 && i <= 181 {
			assert.True(p.Pass(start, current))
		} else {
			assert.False(p.Pass(start, current))
		}
	}

	p, err = Parse("-1m/10s")
	require.NoError(err)
	for i := 0; i < 100; i++ {
		current := start.Add(time.Second * time.Duration(i))
		if i%10 == 0 && i <= 60 && i > 0 {
			assert.True(p.Pass(start, current))
		} else {
			assert.False(p.Pass(start, current))
		}
	}

	p, err = Parse("23s-/10s")
	require.NoError(err)
	for i := 0; i < 100; i++ {
		current := start.Add(time.Second * time.Duration(i))
		if i%10 == 0 && i >= 23 && i > 0 {
			assert.True(p.Pass(start, current))
		} else {
			assert.False(p.Pass(start, current))
		}
	}

	p, err = Parse("10m-1h/10m, 1h-/30m")
	require.NoError(err)
	assert.True(p.Pass(start, start.Add(time.Minute*10)))
	assert.False(p.Pass(start, start.Add(time.Minute*11)))
}

func TestNextTime(t *testing.T) {
	assert, require := test.Assert(t), test.Require(t)
	p, err := Parse("0s,*/10s")
	require.NoError(err)
	start := time.Date(2020, 8, 28, 15, 2, 47, 0, time.Local)

	now := time.Now().Truncate(time.Second)
	for i := 0; i < 80; i++ {
		current := now.Add(time.Second * time.Duration(i))
		next := p.NextTime(start, current)
		assert.True(next.Second()%10 == 0)
	}
}

func TestNextTime2(t *testing.T) {
	require := test.Require(t)
	p, err := Parse("5s,10m")
	require.NoError(err)
	start := time.Date(2022, 10, 18, 14, 7, 21, 0, time.Local)
	t.Log(p.NextTime(start, time.Now()))
}

func TestNextTime3(t *testing.T) {
	assert, require := test.Assert(t), test.Require(t)
	p, err := Parse("@10m")
	require.NoError(err)
	assert.Equal(
		t,
		p.NextTime(time.Now(), time.Date(2022, 10, 18, 14, 10, 0, 0, time.Local)),
		time.Date(2022, 10, 18, 14, 20, 0, 0, time.Local),
	)
	assert.Equal(
		t,
		p.NextTime(time.Now(), time.Date(2022, 10, 18, 14, 13, 3, 0, time.Local)),
		time.Date(2022, 10, 18, 14, 20, 0, 0, time.Local),
	)
}

func TestRangeRegexp(t *testing.T) {
	assert := test.Assert(t)
	assert.True(rangeRegexp.MatchString("-1h/23m"))
	assert.True(rangeRegexp.MatchString("10m-1h/23m"))
	assert.True(rangeRegexp.MatchString("1h-/23m"))
	assert.False(rangeRegexp.MatchString("10m-1h/"))
	assert.False(rangeRegexp.MatchString("10m-1h"))
	assert.False(rangeRegexp.MatchString("23m"))
	assert.False(rangeRegexp.MatchString("*/23m"))

	submatch := rangeRegexp.FindStringSubmatch("-1h/23m")
	assert.Equal("", submatch[1])
	assert.Equal("1h", submatch[2])
	assert.Equal("23m", submatch[3])
	submatch = rangeRegexp.FindStringSubmatch("10m-1h/23m")
	assert.Equal("10m", submatch[1])
	assert.Equal("1h", submatch[2])
	assert.Equal("23m", submatch[3])
	submatch = rangeRegexp.FindStringSubmatch("1h-/23m")
	assert.Equal("1h", submatch[1])
	assert.Equal("", submatch[2])
	assert.Equal("23m", submatch[3])
}

func TestLongTime(t *testing.T) {
	assert := test.Assert(t)
	t.Run("longTime1", func(t *testing.T) {
		start, _ := time.Parse(time.RFC3339, "2021-08-07T11:38:49+08:00")
		now, _ := time.Parse(time.RFC3339, "2021-08-10T08:47:09+08:00")
		now2, err := time.Parse(time.RFC3339, "2021-08-10T08:47:11+08:00")
		assert.NoError(err)
		p, err := Parse("5s,*/10s")
		assert.NoError(err)
		assert.True(p.Pass(start, now), now.Sub(start))
		assert.False(p.Pass(start, now2), now2.Sub(start))
	})
	t.Run("longTime2", func(t *testing.T) {
		start, _ := time.Parse(time.RFC3339, "2021-09-24T10:25:46+08:00")
		now, _ := time.Parse(time.RFC3339, "2021-09-24T10:35:46+08:00")
		p, err := Parse("-1h/10m")
		assert.NoError(err)
		assert.False(p.Pass(start.Add(time.Hour*12).Truncate(time.Second), now), now.Sub(start))
	})
	t.Run("normalTime1", func(t *testing.T) {
		start, _ := time.Parse(time.RFC3339, "2021-10-05T00:26:30+08:00")
		now, _ := time.Parse(time.RFC3339, "2021-10-05T00:30:30+08:00")
		p, err := Parse("-30m/2m,-1h/5m, 1h-/30m,24h-/24h")
		assert.NoError(err)
		assert.True(p.Pass(start.Add(time.Minute*2).Truncate(time.Second), now), now.Sub(start))
	})

}
