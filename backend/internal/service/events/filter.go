package events

import "github.com/sklyar-vlad/umirHack-flagman/internal/model"

const (
	roleAdmin         = "admin"
	roleManager       = "manager"
	roleStorekeeper   = "storekeeper"
	roleOrderOperator = "order_operator"
)

// Reconstruct the wire payload from allowlisted fields. Never serialize stored
// JSON or internal correlation data directly, even for administrators.
func Filter(e *model.PublishedEvent, role string) (model.OutboxPayload, bool) {
	p := model.OutboxPayload{OccurredAt: e.Payload.OccurredAt, Topics: []model.EventTopic{}}
	switch e.Type {
	case model.IncomingSuppliesChanged, model.InventoryChanged,
		model.InventoryTimeChanged,
		model.ProductsChanged,
		model.CategoriesChanged,
		model.DishesChanged,
		model.SettingsChanged,
		model.UsersChanged:
	default:
		return p, false
	}
	allowed := func(t model.EventTopic) bool {
		common := role == roleAdmin || role == roleManager || role == roleStorekeeper || role == roleOrderOperator
		switch t {
		case model.TopicInventory,
			model.TopicBatches,
			model.TopicDashboard,
			model.TopicProducts,
			model.TopicProductCategories,
			model.TopicSettings:
			return common
		case model.TopicDishes, model.TopicStopList, model.TopicHotOffers, model.TopicOrders:
			return role == roleAdmin || role == roleManager || role == roleOrderOperator
		case model.TopicSupplies, model.TopicStockMovements, model.TopicProcurementRecommendations:
			return role == roleAdmin || role == roleManager || role == roleStorekeeper
		case model.TopicUsers:
			return role == roleAdmin
		default:
			return false
		}
	}
	seen := map[model.EventTopic]bool{}
	for _, t := range e.Payload.Topics {
		if allowed(t) && !seen[t] {
			p.Topics = append(p.Topics, t)
			seen[t] = true
		}
	}
	if len(p.Topics) == 0 {
		return p, false
	}
	if seen[model.TopicInventory] || seen[model.TopicProducts] {
		p.ProductIDs = e.Payload.ProductIDs
	}
	if seen[model.TopicBatches] {
		p.BatchIDs = e.Payload.BatchIDs
	}
	if seen[model.TopicDishes] {
		p.DishIDs = e.Payload.DishIDs
	}
	if seen[model.TopicProductCategories] {
		p.CategoryIDs = e.Payload.CategoryIDs
	}
	if seen[model.TopicSupplies] {
		p.SupplyIDs = e.Payload.SupplyIDs
	}
	if seen[model.TopicOrders] {
		p.OrderIDs = e.Payload.OrderIDs
	}
	if seen[model.TopicUsers] {
		p.UserIDs = e.Payload.UserIDs
	}
	return p, true
}
