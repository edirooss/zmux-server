package principal

import "github.com/gin-gonic/gin"

const principalKey = "auth.principal"

func SetPrincipal(c *gin.Context, id string, authType AuthType, kind Kind) {
	c.Set(principalKey, &Principal{
		ID:       id,
		AuthType: authType,
		Kind:     kind,
	})
}

func GetPrincipal(c *gin.Context) *Principal {
	if v, ok := c.Get(principalKey); ok {
		if p, ok := v.(*Principal); ok {
			return p
		}
	}
	return nil
}
