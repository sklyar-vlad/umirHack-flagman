package events

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
)

func TestRoleFilteringAllowlistAndIdentifiers(t *testing.T) {
	id := uuid.New()
	e := model.PublishedEvent{
		Type: model.InventoryChanged,
		Payload: model.OutboxPayload{
			RequestID: "private correlation",
			Topics: []model.EventTopic{
				model.TopicInventory,
				model.TopicBatches,
				model.TopicDashboard,
				model.TopicDishes,
				model.TopicHotOffers,
				model.TopicStopList,
				model.TopicProcurementRecommendations,
				model.TopicSupplies,
				model.TopicOrders,
				model.TopicStockMovements,
				model.TopicUsers,
				"unknown",
			},
			ProductIDs: []uuid.UUID{id},
			BatchIDs:   []uuid.UUID{id},
			DishIDs:    []uuid.UUID{id},
			SupplyIDs:  []uuid.UUID{id},
			OrderIDs:   []uuid.UUID{id},
			UserIDs:    []uuid.UUID{id},
		},
	}
	for _, role := range []string{"admin", "manager", "storekeeper", "order_operator", "unknown"} {
		p, ok := Filter(&e, role)
		if ok != (role != "unknown") {
			t.Fatal(role, ok)
		}
		raw, _ := json.Marshal(p)
		if strings.Contains(string(raw), "private") || strings.Contains(string(raw), "unknown") {
			t.Fatal("unsafe payload", string(raw))
		}
		menu := role == "admin" || role == "manager" || role == "order_operator"
		warehouse := role == "admin" || role == "manager" || role == "storekeeper"
		if (len(p.DishIDs) > 0) != menu || (len(p.OrderIDs) > 0) != menu || (len(p.SupplyIDs) > 0) != warehouse ||
			(len(p.UserIDs) > 0) != (role == "admin") {
			t.Fatal(role, p)
		}
	}
	e.Type = model.UsersChanged
	e.Payload.Topics = []model.EventTopic{model.TopicUsers}
	for _, role := range []string{"admin", "manager", "storekeeper", "order_operator"} {
		_, ok := Filter(&e, role)
		if ok != (role == "admin") {
			t.Fatal("users leak", role)
		}
	}
	e.Type = "unknown.event"
	if _, ok := Filter(&e, "admin"); ok {
		t.Fatal("unknown event exposed")
	}
}
