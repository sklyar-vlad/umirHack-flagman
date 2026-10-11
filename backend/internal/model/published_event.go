package model

type PublishedEvent struct {
	Cursor  int64
	Type    OutboxEventType
	Payload OutboxPayload
}
type EventPage struct {
	HighWater      int64
	RetentionFloor int64
	Events         []PublishedEvent
}
