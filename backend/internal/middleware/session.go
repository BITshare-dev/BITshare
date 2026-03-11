package middleware

import (
	"errors"

	"github.com/gin-gonic/gin"

	"openshare/backend/internal/session"
)

func SessionLoader(manager *session.Manager) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		cookieValue, err := ctx.Cookie(managerCookieName(manager))
		if err != nil {
			ctx.Next()
			return
		}

		result, resolveErr := manager.Resolve(ctx.Request.Context(), cookieValue)
		if resolveErr != nil {
			if result != nil && result.ShouldClear {
				manager.ClearCookie(ctx.Writer)
			}
			if errors.Is(resolveErr, session.ErrNoSession) ||
				errors.Is(resolveErr, session.ErrInvalidSession) ||
				errors.Is(resolveErr, session.ErrExpiredSession) ||
				errors.Is(resolveErr, session.ErrInactiveAdmin) {
				ctx.Next()
				return
			}

			ctx.Error(resolveErr)
			ctx.Next()
			return
		}

		session.SetAdminIdentity(ctx, result.Identity)
		if result.Renewed {
			manager.WriteCookie(ctx.Writer, result.CookieValue, result.Identity.ExpiresAt)
		}

		ctx.Next()
	}
}

func managerCookieName(manager cookieNameProvider) string {
	return manager.CookieName()
}

type cookieNameProvider interface {
	CookieName() string
}
