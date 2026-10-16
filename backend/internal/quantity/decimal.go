package quantity

import (
	"fmt"
	"math/big"
	"strings"
)

func ParseMilli(raw string) (*big.Int, error) {
	whole, fraction, hasFraction := strings.Cut(raw, ".")
	if whole == "" || len(fraction) > 3 || (hasFraction && fraction == "") {
		return nil, fmt.Errorf("invalid stock quantity %q", raw)
	}
	for _, digit := range whole + fraction {
		if digit < '0' || digit > '9' {
			return nil, fmt.Errorf("invalid stock quantity %q", raw)
		}
	}
	value, ok := new(big.Int).SetString(whole+fraction+strings.Repeat("0", 3-len(fraction)), 10)
	if !ok {
		return nil, fmt.Errorf("invalid stock quantity %q", raw)
	}
	return value, nil
}

func FormatMilli(value *big.Int) string {
	digits := value.String()
	if len(digits) <= 3 {
		digits = strings.Repeat("0", 4-len(digits)) + digits
	}
	return digits[:len(digits)-3] + "." + digits[len(digits)-3:]
}
