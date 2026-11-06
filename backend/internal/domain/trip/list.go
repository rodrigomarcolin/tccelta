package trip

import (
	"encoding/base64"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
)

// Limites da listagem (GET /v1/viagens).
const (
	DefaultListLimit = 24
	MaxListLimit     = 50
	MaxSearchRunes   = 60
)

// ListCursor é a posição do último item entregue: a listagem segue (created_at, trip_id) em ordem decrescente.
type ListCursor struct {
	CreatedAt time.Time
	ID        uuid.UUID
}

// Encode devolve o cursor opaco. O created_at vai em RFC 3339 com nanossegundos, para não perder os microssegundos do banco.
func (c ListCursor) Encode() string {
	raw := c.CreatedAt.UTC().Format(time.RFC3339Nano) + "|" + c.ID.String()
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

// ParseListCursor lê um cursor devolvido por Encode; qualquer outra coisa é FieldError.
func ParseListCursor(s string) (ListCursor, error) {
	bad := FieldError{"cursor", "inválido"}
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return ListCursor{}, bad
	}
	at, id, ok := strings.Cut(string(raw), "|")
	if !ok || len(id) != 36 {
		return ListCursor{}, bad
	}
	t, err := time.Parse(time.RFC3339Nano, at)
	if err != nil {
		return ListCursor{}, bad
	}
	u, err := uuid.Parse(id)
	if err != nil {
		return ListCursor{}, bad
	}
	return ListCursor{CreatedAt: t.UTC(), ID: u}, nil
}

// ListQuery é o pedido de listagem já validado.
type ListQuery struct {
	Limit  int
	Cursor *ListCursor
	// Search é o trecho do nome do motorista, já normalizado (NormalizeName); vazio não filtra.
	Search string
}

// NewListQuery valida os parâmetros da listagem. limit nulo vale DefaultListLimit; cursor e search vazios não valem nada.
// O erro é um FieldError.
func NewListQuery(limit *int64, cursor, search string) (ListQuery, error) {
	q := ListQuery{Limit: DefaultListLimit}
	if limit != nil {
		if *limit < 1 || *limit > MaxListLimit {
			return ListQuery{}, FieldError{"limite", fmt.Sprintf("de 1 a %d", MaxListLimit)}
		}
		q.Limit = int(*limit)
	}
	if cursor != "" {
		c, err := ParseListCursor(cursor)
		if err != nil {
			return ListQuery{}, err
		}
		q.Cursor = &c
	}
	s := strings.TrimSpace(search)
	if utf8.RuneCountInString(s) > MaxSearchRunes {
		return ListQuery{}, FieldError{"busca", fmt.Sprintf("até %d caracteres", MaxSearchRunes)}
	}
	if strings.ContainsFunc(s, unicode.IsControl) {
		return ListQuery{}, FieldError{"busca", "não pode ter caracteres de controle"}
	}
	q.Search = NormalizeName(s)
	return q, nil
}

// ListItem é uma viagem da listagem, com o que o resumo da análise informa.
type ListItem struct {
	Trip
	// StartedAt e EndedAt, DistanceKm e DurationS vêm de gold.trip_summary (nulos antes de a viagem ficar pronta).
	StartedAt, EndedAt    *time.Time
	DistanceKm, DurationS *float64
	// Route é o resumo da rota (RoutePreview); nulo sem GPS ou antes de a viagem ficar pronta.
	Route [][]Point
}

// Period devolve o início e o fim da viagem: os do resumo ou, sem eles, os da entrada por intervalo; senão nulos.
func (i ListItem) Period() (start, end *time.Time) {
	if i.StartedAt != nil && i.EndedAt != nil {
		return i.StartedAt, i.EndedAt
	}
	if i.EntryType == EntryTypeInterval {
		return i.IntervalStart, i.IntervalEnd
	}
	return nil, nil
}

// Page é um lote da listagem. Next é nulo na última página.
type Page struct {
	Items []ListItem
	// Total conta todas as viagens que casam com a busca, não só as da página.
	Total int64
	Next  *ListCursor
}

// PageOf corta as linhas buscadas (até Limit+1) na página: a linha a mais só diz que há próxima.
func PageOf(items []ListItem, total int64, limit int) Page {
	p := Page{Items: items, Total: total}
	if len(items) > limit {
		p.Items = items[:limit]
		last := p.Items[limit-1]
		p.Next = &ListCursor{CreatedAt: last.CreatedAt, ID: last.ID}
	}
	if p.Items == nil {
		p.Items = []ListItem{}
	}
	return p
}
