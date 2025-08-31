package principal

import "github.com/gin-gonic/gin"

const principalKey = "auth.principal"

func SetPrincipal(c *gin.Context, p *Principal) {
	c.Set(principalKey, p)
}

func GetPrincipal(c *gin.Context) *Principal {
	if v, ok := c.Get(principalKey); ok {
		if p, ok := v.(*Principal); ok {
			return p
		}
	}
	return nil
}
