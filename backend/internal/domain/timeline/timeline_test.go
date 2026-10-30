package timeline

import (
	"math"
	"math/rand"
	"testing"
	"time"
)

func ms(h, m int, s float64) float64 {
	return float64(time.Date(2026, 10, 26, h, m, 0, 0, time.UTC).UnixMilli()) + s*1000
}

func TestDocExample(t *testing.T) {
	c := DefaultConfig()
	e := EstimateFromAnchors([]Anchor{
		{TMs: 600000, S1Ms: ms(10, 0, 0), S2Ms: ms(10, 0, 0.8)},
		{TMs: 900000, S1Ms: ms(10, 5, 0.4), S2Ms: ms(10, 5, 1.0)},
	}, c)
	if len(e.Groups) != 1 || e.B != 0 || e.BaseUncertaintyMs != 200 {
		t.Fatalf("%v", e)
	}
	g := e.Groups[0]
	if g.Lo != ms(9, 50, 0.4) || g.Hi != ms(9, 50, 0.8) || e.AMs != ms(9, 50, 0.6) {
		t.Fatalf("lo=%v hi=%v a=%v", g.Lo, g.Hi, e.AMs)
	}
}

func TestEmptyIntersection(t *testing.T) {
	e := EstimateFromAnchors([]Anchor{
		{TMs: 0, S1Ms: 1000, S2Ms: 1100},
		{TMs: 60000, S1Ms: 61500, S2Ms: 61600},
	}, DefaultConfig())
	if len(e.Groups) != 2 || len(e.Alerts) != 1 || e.Alerts[0].Type != "empty_intersection" {
		t.Fatalf("%v %v", e, e.Alerts)
	}
}

func synthetic(ppm float64, hours int, noise float64, seed int64) []Anchor {
	r := rand.New(rand.NewSource(seed)) //nolint:gosec // G404: semente fixa de teste
	var out []Anchor
	for t := int64(0); t <= int64(hours)*3600000; t += 1800000 {
		o := 1.7e12 + ppm*1e-6*float64(t)
		out = append(out, Anchor{TMs: t, S1Ms: o + float64(t) - noise*r.Float64(), S2Ms: o + float64(t) + noise*r.Float64()})
	}
	return out
}

func TestKnownDrift(t *testing.T) {
	e := EstimateFromAnchors(synthetic(20, 6, 100, 1), DefaultConfig())
	if math.Abs(e.B*1e6-20) > 5 || len(e.Alerts) != 0 {
		t.Fatalf("%v %v", e, e.Alerts)
	}
}

func TestExcessiveDrift(t *testing.T) {
	e := EstimateFromAnchors(synthetic(60, 6, 100, 2), DefaultConfig())
	found := false
	for _, a := range e.Alerts {
		found = found || a.Type == "excessive_drift"
	}
	if !found {
		t.Fatalf("%v", e)
	}
}

func TestNearbyGroups(t *testing.T) {
	e := EstimateFromAnchors([]Anchor{
		{TMs: 0, S1Ms: 1000, S2Ms: 1400},
		{TMs: 700000, S1Ms: 701300, S2Ms: 701700},
	}, DefaultConfig())
	if len(e.Groups) != 2 || e.B != 0 || len(e.Alerts) != 0 {
		t.Fatalf("%v %v", e, e.Alerts)
	}
}

func TestWindowDoesNotChain(t *testing.T) {
	var a []Anchor
	for i := int64(0); i <= 15; i++ {
		a = append(a, Anchor{TMs: i * 240000, S1Ms: 1e12, S2Ms: 1e12 + 100})
		a[i].S1Ms += float64(a[i].TMs)
		a[i].S2Ms += float64(a[i].TMs)
	}
	g, _ := GroupAnchors(a, DefaultConfig())
	for _, x := range g {
		if x.N > 3 {
			t.Fatalf("grupo com %d", x.N)
		}
	}
}

func TestNeighbors(t *testing.T) {
	c := DefaultConfig()
	p := &NeighborReference{AMs: 1000, BaseUncertaintyMs: 100, FirstTMs: 0, LastTMs: 3600000}
	n := &NeighborReference{AMs: 1000, BaseUncertaintyMs: 100, FirstTMs: 20000000, LastTMs: 21000000}
	f := BootRange{FirstTMs: 5000000, LastTMs: 6000000}
	e := EstimateFromNeighbors(p, n, f, nil, c)
	if e.Source != "neighbors" {
		t.Fatalf("%v", e)
	}
	d := 50.0
	e2 := EstimateFromNeighbors(p, nil, f, &d, c)
	if e2.Source != "" && e2.Source != "neighbors" {
		t.Fatal(e2)
	}
	if e3 := EstimateFromNeighbors(nil, n, f, &d, c); e3.Source != "" {
		t.Fatal("sem anterior")
	}
	if e4 := EstimateFromNeighbors(p, nil, f, nil, c); e4.Source != "" {
		t.Fatal("sem limite superior")
	}
	low := -1e9
	if e5 := EstimateFromNeighbors(p, nil, f, &low, c); len(e5.Alerts) != 1 {
		t.Fatal("lo > hi")
	}
}

func TestUnwrap(t *testing.T) {
	const v = int64(1) << 32
	if Unwrap(5, -1) != 5 {
		t.Fatal()
	}
	if got := Unwrap(10, v-100); got != v+10 {
		t.Fatalf("%d", got)
	}
	if got := Unwrap(uint32(v-50), v+20); got != v-50 {
		t.Fatalf("%d", got)
	}
	if got := Unwrap(100, 50); got != 100 {
		t.Fatalf("%d", got)
	}
}

func TestUncertainty(t *testing.T) {
	g := []Group{{Lo: 0, Hi: 400, TCenter: 0, N: 1}, {Lo: 0, Hi: 200, TCenter: 1e7, N: 1}}
	u := Uncertainty(1e7+1000, g, DefaultConfig())
	if math.Abs(u-(100+40e-6*1000)) > 1e-9 {
		t.Fatal(u)
	}
}
