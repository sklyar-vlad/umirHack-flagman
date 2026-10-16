package httpjson

import (
	"strings"
	"testing"
)

func TestDecodeObjectRejectsAmbiguousNestedIntegrationWrites(t *testing.T) {
	type line struct {
		Quantity string `json:"quantity"`
	}
	type body struct {
		Items []line `json:"items"`
	}
	for _, raw := range []string{`null`, `[]`, `{"items":[],"items":[]}`, `{"items":[{"quantity":"1","quantity":"9"}]}`, `{"Items":[]}`, `{"items":[]} {}`, `{"items":[{"quantity":2}]}`, `{"secret":"x"}`} {
		var result body
		if err := DecodeObject(strings.NewReader(raw), &result); err == nil {
			t.Fatalf("accepted ambiguous body %s", raw)
		}
	}
	var result body
	if err := DecodeObject(
		strings.NewReader(`{"items":[{"quantity":"1.001"}]}`),
		&result,
	); err != nil || len(result.Items) != 1 ||
		result.Items[0].Quantity != "1.001" {
		t.Fatal(result, err)
	}
}
