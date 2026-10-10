package requestctx

import (
	"context"

	"github.com/google/uuid"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
)

type actorKey struct{}

func WithActor(ctx context.Context, actor model.Actor) context.Context {
	return context.WithValue(ctx, actorKey{}, actor)
}

func Actor(ctx context.Context) (model.Actor, bool) {
	actor, ok := ctx.Value(actorKey{}).(model.Actor)
	return actor, ok && actor.ID != uuid.Nil
}
