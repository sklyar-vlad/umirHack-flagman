package hotoffers

import (
	"math/big"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/sklyar-vlad/umirHack-flagman/internal/fefo"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
)

func scoreFixture(id uuid.UUID, expiry time.Time, remaining, used string) simulatedOffer {
	return simulatedOffer{dish: model.Dish{ID: id}, problems: []problemBatch{{allocation: fefo.Allocation{
		Batch: model.OrderBatch{ExpiresAt: expiry, RemainingQuantity: remaining}, Quantity: used,
	}}}}
}

func TestScoresExactMaximumAndUrgency(t *testing.T) {
	now := time.Now().UTC()
	offer := scoreFixture(uuid.New(), now.Add(18*time.Hour), "1800.000", "120.000")
	got, err := scoreOffers([]simulatedOffer{offer}, now, 24)
	if err != nil || len(got) != 1 || got[0].score.Cmp(big.NewRat(5, 3)) != 0 ||
		got[0].score.FloatString(4) != "1.6667" ||
		got[0].level != "low" {
		t.Fatalf("exact sample: %+v, %v", got, err)
	}
	offer = scoreFixture(uuid.New(), now.Add(6*time.Hour), "3.000", "1.000")
	later := scoreFixture(uuid.New(), now.Add(12*time.Hour), "5.000", "3.000")
	offer.problems = append(offer.problems, later.problems...)
	got, err = scoreOffers([]simulatedOffer{offer}, now, 24)
	if err != nil || got[0].score.Cmp(big.NewRat(30, 1)) != 0 || got[0].level != "high" ||
		!got[0].nearest.Equal(now.Add(6*time.Hour)) {
		t.Fatalf("maximum and nearest differ: %+v, %v", got, err)
	}
	for _, tc := range []struct {
		h     time.Duration
		level string
	}{
		{time.Nanosecond, "high"},
		{6 * time.Hour, "high"},
		{6*time.Hour + time.Nanosecond, "medium"},
		{12 * time.Hour, "medium"},
		{12*time.Hour + time.Nanosecond, "low"},
		{24 * time.Hour, "low"},
	} {
		got, err := scoreOffers([]simulatedOffer{scoreFixture(uuid.New(), now.Add(tc.h), "1.000", "1.000")}, now, 24)
		if err != nil || got[0].level != tc.level {
			t.Fatalf("urgency at %s: %+v, %v", tc.h, got, err)
		}
		if tc.h == 24*time.Hour && got[0].score.Sign() != 0 {
			t.Fatal("threshold score must be zero")
		}
	}
	before, _ := scoreOffers(
		[]simulatedOffer{scoreFixture(uuid.New(), now.Add(12*time.Hour), "3.000", "1.000")},
		now,
		24,
	)
	after, _ := scoreOffers(
		[]simulatedOffer{scoreFixture(uuid.New(), now.Add(12*time.Hour), "3.000", "1.000")},
		now.Add(time.Nanosecond),
		24,
	)
	if after[0].score.Cmp(before[0].score) <= 0 {
		t.Fatal("shorter remaining time must increase exact score")
	}
}

func TestScoresSortWithoutDisplayRounding(t *testing.T) {
	now := time.Now().UTC()
	id1 := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	id2 := uuid.MustParse("00000000-0000-0000-0000-000000000002")
	low := scoreFixture(id1, now.Add(12*time.Hour), "100000000.000", "1.000")
	high := scoreFixture(id2, now.Add(13*time.Hour), "100000000.000", "1.100")
	got, err := scoreOffers([]simulatedOffer{low, high}, now, 24)
	if err != nil || got[0].dish.ID != id2 || got[0].score.FloatString(4) != got[1].score.FloatString(4) {
		t.Fatalf("rounded sort: %+v, %v", got, err)
	}

	a := scoreFixture(id2, now.Add(12*time.Hour), "2.000", "1.000")
	b := scoreFixture(id1, now.Add(18*time.Hour), "1.000", "1.000")
	c := scoreFixture(id1, now.Add(12*time.Hour), "2.000", "1.000")
	got, err = scoreOffers([]simulatedOffer{b, a, c}, now, 24)
	if err != nil || got[0].dish.ID != id1 || got[1].dish.ID != id2 || !got[2].nearest.Equal(now.Add(18*time.Hour)) {
		t.Fatalf("stable ties: %+v, %v", got, err)
	}
}

func TestScoresRejectInvalidFacts(t *testing.T) {
	now := time.Now().UTC()
	for _, tc := range []struct {
		h               time.Duration
		remaining, used string
	}{
		{0, "1", "1"},
		{24*time.Hour + time.Nanosecond, "1", "1"},
		{time.Hour, "0", "1"},
		{time.Hour, "broken", "1"},
		{time.Hour, "1", "0"},
		{time.Hour, "1", "2"},
		{time.Hour, "1", "0.0001"},
	} {
		if _, err := scoreOffers(
			[]simulatedOffer{scoreFixture(uuid.New(), now.Add(tc.h), tc.remaining, tc.used)},
			now,
			24,
		); err == nil {
			t.Fatalf("accepted corrupt facts: %+v", tc)
		}
	}
	if _, err := scoreOffers([]simulatedOffer{{}}, now, 24); err == nil {
		t.Fatal("accepted empty problems")
	}
	if _, err := scoreOffers(nil, now, 721); err == nil {
		t.Fatal("accepted invalid threshold")
	}
}
