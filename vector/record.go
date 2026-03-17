package vector

import "github.com/FrogoAI/lsh/repositories"

const (
	binVector    = "v"
	binGroup     = "g"
	binSignature = "s"
)

type Record struct {
	ID        string
	Vector    []float64
	GroupID   string
	Signature []uint64
}

func (r Record) toBins() map[string]any {
	return map[string]any{
		binVector:    r.Vector,
		binGroup:     r.GroupID,
		binSignature: r.Signature,
	}
}

func recordFromBins(rec repositories.Record) (Record, bool) {
	group, _ := rec.Bins[binGroup].(string)

	var vec []float64

	switch raw := rec.Bins[binVector].(type) {
	case []float64:
		vec = raw
	case []any:
		vec = make([]float64, len(raw))

		for i, v := range raw {
			switch n := v.(type) {
			case float64:
				vec[i] = n
			case int:
				vec[i] = float64(n)
			case int64:
				vec[i] = float64(n)
			}
		}
	}

	var sig []uint64

	switch raw := rec.Bins[binSignature].(type) {
	case []uint64:
		sig = raw
	case []any:
		sig = make([]uint64, len(raw))

		for i, v := range raw {
			switch n := v.(type) {
			case int:
				sig[i] = uint64(n)
			case int64:
				sig[i] = uint64(n)
			case float64:
				sig[i] = uint64(n)
			}
		}
	}

	if len(vec) == 0 {
		return Record{}, false
	}

	return Record{
		ID:        rec.Key,
		Vector:    vec,
		GroupID:   group,
		Signature: sig,
	}, true
}
