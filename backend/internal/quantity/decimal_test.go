package quantity

import "testing"

func TestParseAndFormatMilli(t *testing.T) {
	for _, tc := range []struct{ input, scaled, formatted string }{
		{"0", "0", "0.000"}, {"0.001", "1", "0.001"}, {"1.2", "1200", "1.200"},
		{"001.230", "1230", "1.230"}, {"99999999999.999", "99999999999999", "99999999999.999"},
		{"199999999999998.000", "199999999999998000", "199999999999998.000"},
	} {
		t.Run(tc.input, func(t *testing.T) {
			value, err := ParseMilli(tc.input)
			if err != nil || value.String() != tc.scaled || FormatMilli(value) != tc.formatted {
				t.Fatalf("exact quantity %s: %v %v", tc.input, value, err)
			}
		})
	}
	for _, input := range []string{"", ".001", "1.", "1.0001", "1.2.3", "-1.000", "+1.000", "1e3", " 1.000", "1.000 ", "NaN"} {
		if _, err := ParseMilli(input); err == nil {
			t.Errorf("accepted invalid quantity %q", input)
		}
	}
}
