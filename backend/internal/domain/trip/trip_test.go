package trip

import (
	"testing"
	"time"

	"tccelta/backend/internal/domain/tripanalysis"
)

func i32(v int32) *int32 { return &v }

func TestEntry(t *testing.T) {
	boot := Trip{DongleID: 3, EntryType: "boot", BootGeneration: i32(1), BootID: i32(8)}.Entry()
	if boot.Type != tripanalysis.EntryBoot || boot.DongleID != 3 || *boot.BootID != 8 || *boot.BootGeneration != 1 || boot.Validate() != nil {
		t.Fatalf("%+v", boot)
	}
	t0 := time.Date(2026, 10, 28, 8, 0, 0, 0, time.UTC)
	t1 := t0.Add(time.Hour)
	iv := Trip{DongleID: 3, EntryType: "interval", IntervalStart: &t0, IntervalEnd: &t1}.Entry()
	if iv.Type != tripanalysis.EntryInterval || !iv.T0.Equal(t0) || !iv.T1.Equal(t1) || iv.BootID != nil || iv.Validate() != nil {
		t.Fatalf("%+v", iv)
	}
	// boot_generation nula (o boot ainda não chegou) continua sendo uma entrada válida
	if e := (Trip{EntryType: "boot", BootID: i32(2)}).Entry(); e.Validate() != nil || e.BootGeneration != nil {
		t.Fatalf("%+v", e)
	}
}

func TestBootTime(t *testing.T) {
	a, b, src := 1000.0, 0.001, "anchors"
	boot := Boot{Generation: 1, BootID: 2, AMs: &a, B: &b, TimeSource: &src}
	if !boot.HasTime() || boot.Ref() != (BootRef{1, 2}) || boot.EpochMs(2000) != 1000+1.001*2000 {
		t.Fatalf("%+v %v", boot, boot.EpochMs(2000))
	}
	for _, none := range []Boot{{}, {AMs: &a, B: &b}, {AMs: &a, TimeSource: &src}} {
		if none.HasTime() {
			t.Fatalf("%+v não tem hora", none)
		}
	}
}
