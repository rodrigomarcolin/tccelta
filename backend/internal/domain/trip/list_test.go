package trip

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestNewListQueryDefaultsAndLimits(t *testing.T) {
	q, err := NewListQuery(nil, "", "")
	if err != nil || q.Limit != DefaultListLimit || q.Cursor != nil || q.Search != "" {
		t.Fatalf("%v %+v", err, q)
	}
	for _, ok := range []int64{1, 24, 50} {
		if q, err := NewListQuery(&ok, "", ""); err != nil || q.Limit != int(ok) {
			t.Fatalf("%d: %v", ok, err)
		}
	}
	for _, bad := range []int64{0, -1, 51, 1 << 40} {
		var fe FieldError
		if _, err := NewListQuery(&bad, "", ""); !errors.Is(err, ErrInvalid) || !errors.As(err, &fe) || fe.Field != "limite" {
			t.Fatalf("%d: %v", bad, err)
		}
	}
}

func TestNewListQuerySearch(t *testing.T) {
	q, err := NewListQuery(nil, "", "  JOÃO  ")
	if err != nil || q.Search != "joão" {
		t.Fatalf("%v %q", err, q.Search)
	}
	if _, err := NewListQuery(nil, "", strings.Repeat("ã", MaxSearchRunes)); err != nil {
		t.Fatal(err) // 60 runes, mais de 60 bytes
	}
	for _, bad := range []string{strings.Repeat("a", MaxSearchRunes+1), "a\x00b", "a\nb"} {
		var fe FieldError
		if _, err := NewListQuery(nil, "", bad); !errors.Is(err, ErrInvalid) || !errors.As(err, &fe) || fe.Field != "busca" {
			t.Fatalf("%q: %v", bad, err)
		}
	}
}

func TestListCursorRoundTripKeepsMicroseconds(t *testing.T) {
	c := ListCursor{CreatedAt: time.Date(2026, 10, 28, 17, 3, 0, 123456000, time.FixedZone("x", -3*3600)), ID: uuid.New()}
	got, err := ParseListCursor(c.Encode())
	if err != nil || !got.CreatedAt.Equal(c.CreatedAt) || got.CreatedAt.Nanosecond() != 123456000 || got.ID != c.ID {
		t.Fatalf("%v %+v", err, got)
	}
	q, err := NewListQuery(nil, c.Encode(), "")
	if err != nil || q.Cursor == nil || q.Cursor.ID != c.ID {
		t.Fatalf("%v %+v", err, q)
	}
}

func TestParseListCursorRejectsMalformed(t *testing.T) {
	enc := func(s string) string { return base64.RawURLEncoding.EncodeToString([]byte(s)) }
	id := uuid.NewString()
	for _, bad := range []string{"lixo!", "%%%", enc("sem separador"), enc("2026-10-28T17:03:00Z|abc"), enc("ontem|" + id),
		enc("2026-10-28T17:03:00Z|" + id[:35]), enc("2026-10-28T17:03:00Z|" + id + "x"), enc("|")} {
		if _, err := ParseListCursor(bad); !errors.Is(err, ErrInvalid) {
			t.Errorf("%q: %v", bad, err)
		}
		if _, err := NewListQuery(nil, bad, ""); !errors.Is(err, ErrInvalid) {
			t.Errorf("%q: %v", bad, err)
		}
	}
}

func TestPageOf(t *testing.T) {
	mk := func(n int) []ListItem {
		var out []ListItem
		for i := 0; i < n; i++ {
			out = append(out, ListItem{Trip: Trip{ID: uuid.New(), CreatedAt: time.Unix(int64(1000-i), 0)}})
		}
		return out
	}
	items := mk(4)
	p := PageOf(items, 9, 3)
	if len(p.Items) != 3 || p.Next == nil || p.Next.ID != items[2].ID || !p.Next.CreatedAt.Equal(items[2].CreatedAt) || p.Total != 9 {
		t.Fatalf("%+v", p)
	}
	if p := PageOf(mk(3), 3, 3); len(p.Items) != 3 || p.Next != nil {
		t.Fatalf("última página: %+v", p)
	}
	if p := PageOf(nil, 0, 3); p.Items == nil || len(p.Items) != 0 || p.Next != nil {
		t.Fatalf("vazia: %+v", p)
	}
}

func TestListItemPeriod(t *testing.T) {
	a, b, c, d := time.Unix(1, 0), time.Unix(2, 0), time.Unix(3, 0), time.Unix(4, 0)
	it := ListItem{Trip: Trip{EntryType: EntryTypeInterval, IntervalStart: &c, IntervalEnd: &d}, StartedAt: &a, EndedAt: &b}
	if s, e := it.Period(); s != &a || e != &b {
		t.Fatal("o resumo vale primeiro")
	}
	it.StartedAt, it.EndedAt = nil, nil
	if s, e := it.Period(); s != &c || e != &d {
		t.Fatal("sem resumo, o intervalo")
	}
	it.EntryType = EntryTypeBoot
	if s, e := it.Period(); s != nil || e != nil {
		t.Fatal("boot sem resumo não tem período")
	}
}
