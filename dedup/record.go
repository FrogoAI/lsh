package dedup

import "github.com/FrogoAI/lsh/v2/repositories"

const (
	binInput     = "i"
	binGroup     = "g"
	binSignature = "s"
)

type Record struct {
	ID        string
	Input     string
	GroupID   string
	Signature []uint64
}

func (r Record) toBins() map[string]any {
	return map[string]any{
		binInput:     r.Input,
		binGroup:     r.GroupID,
		binSignature: r.Signature,
	}
}

func recordFromBins(rec repositories.Record) (Record, bool) {
	input, _ := rec.Bins[binInput].(string)
	group, _ := rec.Bins[binGroup].(string)

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

	if input == "" {
		return Record{}, false
	}

	return Record{
		ID:        rec.Key,
		Input:     input,
		GroupID:   group,
		Signature: sig,
	}, true
}
