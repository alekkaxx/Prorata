package prorata

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// goldenFixture is the on-disk format of golden/NN-*.json: a full engine
// input plus the expected invoice.
type goldenFixture struct {
	Name    string  `json:"name"`
	Catalog []Plan  `json:"catalog"`
	Events  []Event `json:"events"`
	Period  Period  `json:"period"`
	Invoice Invoice `json:"invoice"`
}

// TestGolden replays every golden fixture through Compute. Each fixture runs
// twice: the runs must match each other byte-for-byte (the idempotence
// invariant) and match the expected invoice.
func TestGolden(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("golden", "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		t.Run(filepath.Base(file), func(t *testing.T) {
			raw, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			var fx goldenFixture
			if err := json.Unmarshal(raw, &fx); err != nil {
				t.Fatalf("bad fixture: %v", err)
			}

			catalog := Catalog{}
			for _, p := range fx.Catalog {
				catalog[p.ID] = p
			}

			first, err := Compute(catalog, fx.Events, fx.Period)
			if err != nil {
				t.Fatalf("Compute: %v", err)
			}
			second, err := Compute(catalog, fx.Events, fx.Period)
			if err != nil {
				t.Fatalf("Compute (second run): %v", err)
			}

			firstJSON := mustMarshal(t, first)
			if secondJSON := mustMarshal(t, second); !bytes.Equal(firstJSON, secondJSON) {
				t.Fatalf("Compute is not deterministic:\nfirst:  %s\nsecond: %s", firstJSON, secondJSON)
			}
			if wantJSON := mustMarshal(t, fx.Invoice); !bytes.Equal(firstJSON, wantJSON) {
				t.Fatalf("invoice mismatch:\ngot:  %s\nwant: %s", firstJSON, wantJSON)
			}
		})
	}
}

func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
