package tripanalysis

import (
	"math"
	"sort"
)

// median devolve NaN para uma lista vazia.
func median(a []float64) float64 { return quantile(a, 0.5) }

// quantile usa interpolação linear entre amostras ordenadas; NaN para uma lista vazia.
func quantile(a []float64, q float64) float64 {
	if len(a) == 0 {
		return nan
	}
	s := append([]float64(nil), a...)
	sort.Float64s(s)
	pos := float64(len(s)-1) * q
	lo, hi := int(math.Floor(pos)), int(math.Ceil(pos))
	return s[lo] + (s[hi]-s[lo])*(pos-float64(lo))
}

func ptr(x float64) *float64 {
	if isNaN(x) {
		return nil
	}
	return &x
}

func f64(x float64) *float64 { return &x }

func i32(x int) *int32 { v := int32(x); return &v }

// valPtr converte NaN em nil (para as séries expostas).
func valPtr(x float64) *float64 { return ptr(x) }

// haversineKm é a distância entre dois pontos na esfera de R = 6371 km.
func haversineKm(lat1, lon1, lat2, lon2 float64) float64 {
	const r = 6371.0
	rad := math.Pi / 180
	dlat, dlon := (lat2-lat1)*rad, (lon2-lon1)*rad
	s := math.Sin(dlat/2)*math.Sin(dlat/2) + math.Cos(lat1*rad)*math.Cos(lat2*rad)*math.Sin(dlon/2)*math.Sin(dlon/2)
	return 2 * r * math.Asin(math.Min(1, math.Sqrt(s)))
}
