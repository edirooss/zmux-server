package handler

import (
	"net/http"
	"strconv"

	"github.com/edirooss/zmux-server/internal/domain/principal"
	"github.com/edirooss/zmux-server/internal/service"
	"github.com/gin-gonic/gin"
)

func Me(authsvc *service.AuthService, b2bclntsvc *service.B2BClientService) gin.HandlerFunc {
	return func(c *gin.Context) {
		p := authsvc.WhoAmI(c)
		if p == nil {
			c.Status(http.StatusUnauthorized)
			return
		}
		if p.Kind == principal.B2BClient {
			clntID, _ := strconv.ParseInt(p.ID, 10, 64)
			clnt, err := b2bclntsvc.GetOne(clntID)
			if err != nil {
				c.Status(http.StatusUnauthorized)
				return
			}

			res := struct {
				ID     int64  `json:"id"`
				Name   string `json:"name"`
				Quotas struct {
					EnabledChannels struct {
						Quota int64 `json:"quota"`
						Usage int64 `json:"usage"`
					} `json:"enabled_channels"`
					EnabledOutputs []struct {
						Ref   string `json:"ref"`
						Quota int64  `json:"quota"`
						Usage int64  `json:"usage"`
					} `json:"enabled_outputs"`
					OnlineChannels struct {
						Quota int64 `json:"quota"`
						Usage int64 `json:"usage"`
					} `json:"online_channels"`
				} `json:"quotas"`
				ChannelIDs []int64 `json:"channel_ids"`
			}{
				clntID,
				clnt.Name,
				clnt.Quotas,
				clnt.ChannelIDs,
			}

			c.JSON(http.StatusOK, res)
			return
		}

		c.JSON(http.StatusOK, p)
	}
}
