package session

import "github.com/gin-gonic/gin"

const adminIdentityContextKey = "admin_identity"

func SetAdminIdentity(ctx *gin.Context, identity AdminIdentity) {
	ctx.Set(adminIdentityContextKey, identity)
}

func GetAdminIdentity(ctx *gin.Context) (AdminIdentity, bool) {
	value, ok := ctx.Get(adminIdentityContextKey)
	if !ok {
		return AdminIdentity{}, false
	}

	identity, ok := value.(AdminIdentity)
	return identity, ok
}
