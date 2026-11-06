package trip

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func i64(v int64) *int64  { return &v }
func sp(v string) *string { return &v }

func bootInput() Input {
	return Input{DriverName: "  João Silva ", Category: " Feira ", Fuel: "gasolina_e30", EntryType: "boot", DongleID: i64(1), BootID: i64(8)}
}

func intervalInput() Input {
	return Input{DriverName: "Ana", Category: "feira", Fuel: "diesel", EntryType: "intervalo", DongleID: i64(2),
		T0: sp("2026-10-28T08:00:00-03:00"), T1: sp("2026-10-28T18:00:00Z")}
}

func TestNewDraftBoot(t *testing.T) {
	d, err := NewDraft(bootInput())
	if err != nil {
		t.Fatal(err)
	}
	if d.DriverName != "João Silva" || d.DriverNameNormalized != "joão silva" || d.Category != "feira" || d.DongleID != 1 ||
		d.EntryType != EntryTypeBoot || *d.BootID != 8 || d.BootGeneration != nil || d.IntervalStart != nil {
		t.Fatalf("%+v", d)
	}
	in := bootInput()
	in.BootGeneration = i64(65535)
	if d, err := NewDraft(in); err != nil || *d.BootGeneration != 65535 {
		t.Fatalf("%v %+v", err, d)
	}
}

func TestNewDraftInterval(t *testing.T) {
	d, err := NewDraft(intervalInput())
	if err != nil {
		t.Fatal(err)
	}
	if d.EntryType != EntryTypeInterval || d.BootID != nil || d.IntervalStart.Location() != time.UTC ||
		!d.IntervalStart.Equal(time.Date(2026, 10, 28, 11, 0, 0, 0, time.UTC)) || !d.IntervalEnd.Equal(time.Date(2026, 10, 28, 18, 0, 0, 0, time.UTC)) {
		t.Fatalf("%+v", d)
	}
}

func TestNewDraftInvalid(t *testing.T) {
	cases := []struct {
		name  string
		field string
		edit  func(*Input)
	}{
		{"nome vazio", "motorista.nome", func(in *Input) { in.DriverName = "   " }},
		{"nome com 61 runes", "motorista.nome", func(in *Input) { in.DriverName = strings.Repeat("é", 61) }},
		{"nome com controle", "motorista.nome", func(in *Input) { in.DriverName = "a\nb" }},
		{"categoria vazia", "categoria_viagem", func(in *Input) { in.Category = "" }},
		{"categoria com 41 runes", "categoria_viagem", func(in *Input) { in.Category = strings.Repeat("a", 41) }},
		{"categoria com símbolo", "categoria_viagem", func(in *Input) { in.Category = "feira!" }},
		{"personalizado", "combustivel", func(in *Input) { in.Fuel = "personalizado" }},
		{"combustível ausente", "combustivel", func(in *Input) { in.Fuel = "" }},
		{"dongle ausente", "entrada.dongle_id", func(in *Input) { in.DongleID = nil }},
		{"dongle zero", "entrada.dongle_id", func(in *Input) { in.DongleID = i64(0) }},
		{"dongle grande", "entrada.dongle_id", func(in *Input) { in.DongleID = i64(1 << 40) }},
		{"sem tipo", "entrada.tipo", func(in *Input) { in.EntryType = "" }},
		{"tipo desconhecido", "entrada.tipo", func(in *Input) { in.EntryType = "trecho" }},
		{"boot com t0", "entrada", func(in *Input) { in.T0 = sp("2026-10-28T08:00:00Z") }},
		{"boot sem boot_id", "entrada.boot_id", func(in *Input) { in.BootID = nil }},
		{"boot_id 0", "entrada.boot_id", func(in *Input) { in.BootID = i64(0) }},
		{"boot_id 65536", "entrada.boot_id", func(in *Input) { in.BootID = i64(65536) }},
		{"geração negativa", "entrada.boot_generation", func(in *Input) { in.BootGeneration = i64(-1) }},
		{"geração 65536", "entrada.boot_generation", func(in *Input) { in.BootGeneration = i64(65536) }},
		{"intervalo com boot_id", "entrada", func(in *Input) { *in = intervalInput(); in.BootID = i64(1) }},
		{"intervalo com geração", "entrada", func(in *Input) { *in = intervalInput(); in.BootGeneration = i64(0) }},
		{"intervalo sem t0", "entrada.t0", func(in *Input) { *in = intervalInput(); in.T0 = nil }},
		{"intervalo sem t1", "entrada.t1", func(in *Input) { *in = intervalInput(); in.T1 = nil }},
		{"t0 sem fuso", "entrada.t0", func(in *Input) { *in = intervalInput(); in.T0 = sp("2026-10-28T08:00:00") }},
		{"t0 só data", "entrada.t0", func(in *Input) { *in = intervalInput(); in.T0 = sp("2026-10-28") }},
		{"t1 igual a t0", "entrada.t1", func(in *Input) { *in = intervalInput(); in.T1 = in.T0 }},
		{"t1 antes de t0", "entrada.t1", func(in *Input) { *in = intervalInput(); in.T0, in.T1 = in.T1, in.T0 }},
		{"intervalo de 8 dias", "entrada.t1", func(in *Input) {
			*in = intervalInput()
			in.T0, in.T1 = sp("2026-10-01T00:00:00Z"), sp("2026-10-09T00:00:00Z")
		}},
	}
	for _, c := range cases {
		in := bootInput()
		c.edit(&in)
		_, err := NewDraft(in)
		var fe FieldError
		if !errors.Is(err, ErrInvalid) || !errors.As(err, &fe) || fe.Field != c.field {
			t.Errorf("%s: err=%v", c.name, err)
		}
	}
}

func TestNewDraftEdgeValid(t *testing.T) {
	in := intervalInput()
	in.T0, in.T1 = sp("2026-10-01T00:00:00Z"), sp("2026-10-08T00:00:00Z") // exatamente 7 dias
	if _, err := NewDraft(in); err != nil {
		t.Fatal(err)
	}
	in = bootInput()
	in.DriverName, in.Category = strings.Repeat("é", 60), strings.Repeat("a", 40)
	in.BootID, in.BootGeneration = i64(65535), i64(0)
	if _, err := NewDraft(in); err != nil {
		t.Fatal(err)
	}
	in.Category = "Feira 2026_a-b"
	if d, err := NewDraft(in); err != nil || d.Category != "feira 2026_a-b" {
		t.Fatalf("%v %+v", err, d)
	}
}

func TestNormalizeNameAndErrors(t *testing.T) {
	if NormalizeName("  JOÃO ") != NormalizeName("joão") {
		t.Fatal("nomes devem coincidir")
	}
	nr := error(NotReadyError{Status: StatusFailed})
	if !errors.Is(nr, ErrNotReady) || errors.Is(nr, ErrNotFound) || nr.Error() == "" {
		t.Fatalf("%v", nr)
	}
}
