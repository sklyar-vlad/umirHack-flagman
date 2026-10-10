package batch

import "time"

func CalculateStatus(remainingQuantity string, expiresAt, now time.Time, thresholdHours int) (string, int64) {
	seconds := RemainingSeconds(expiresAt, now)
	switch {
	case remainingQuantity == "0.000" || remainingQuantity == "0":
		return "depleted", seconds
	case !expiresAt.After(now):
		return "expired", seconds
	case !expiresAt.After(now.Add(time.Duration(thresholdHours) * time.Hour)):
		return "expiring", seconds
	default:
		return "valid", seconds
	}
}

// RemainingSeconds floors the signed difference without overflowing time.Duration for distant dates.
func RemainingSeconds(expiresAt, now time.Time) int64 {
	seconds := expiresAt.Unix() - now.Unix()
	if expiresAt.Nanosecond() < now.Nanosecond() {
		seconds--
	}
	return seconds
}
