package service

import "context"

type PublicUploadActor struct {
	AdminID          string
	CanDirectPublish bool
}

type publicUploadActorContextKey struct{}

func WithPublicUploadActor(ctx context.Context, actor PublicUploadActor) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, publicUploadActorContextKey{}, actor)
}

func publicUploadActorFromContext(ctx context.Context) (PublicUploadActor, bool) {
	if ctx == nil {
		return PublicUploadActor{}, false
	}

	actor, ok := ctx.Value(publicUploadActorContextKey{}).(PublicUploadActor)
	if !ok || actor.AdminID == "" || !actor.CanDirectPublish {
		return PublicUploadActor{}, false
	}
	return actor, true
}
