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
			clntID, err := strconv.ParseInt(p.ID, 10, 64)
			if err != nil {
				c.Status(http.StatusBadRequest)
				return
			}

			clnt, err := b2bclntsvc.GetOne(clntID)
			if err != nil {
				c.Status(http.StatusUnauthorized)
				return
			}

			// Build enabled outputs
			enabledOutputs := make(map[string]struct {
				Quota int64 `json:"quota"`
				Usage int64 `json:"usage"`
			})
			for _, outputQuota := range clnt.Quotas.EnabledOutputs {
				enabledOutputs[outputQuota.Ref] = struct {
					Quota int64 `json:"quota"`
					Usage int64 `json:"usage"`
				}{
					Quota: outputQuota.Quota,
					Usage: outputQuota.Usage,
				}
			}

			// Prepare the response
			res := struct {
				Name   string `json:"name"`
				Quotas struct {
					EnabledChannels struct {
						Quota int64 `json:"quota"`
						Usage int64 `json:"usage"`
					} `json:"enabled_channels"`
					EnabledOutputs map[string]struct {
						Quota int64 `json:"quota"`
						Usage int64 `json:"usage"`
					} `json:"enabled_outputs"`
					OnlineChannels struct {
						Quota int64 `json:"quota"`
						Usage int64 `json:"usage"`
					} `json:"online_channels"`
				} `json:"quotas"`
				ChannelIDs []int64 `json:"channel_ids"`
			}{
				Name: clnt.Name,
				Quotas: struct {
					EnabledChannels struct {
						Quota int64 `json:"quota"`
						Usage int64 `json:"usage"`
					} `json:"enabled_channels"`
					EnabledOutputs map[string]struct {
						Quota int64 `json:"quota"`
						Usage int64 `json:"usage"`
					} `json:"enabled_outputs"`
					OnlineChannels struct {
						Quota int64 `json:"quota"`
						Usage int64 `json:"usage"`
					} `json:"online_channels"`
				}{
					EnabledChannels: clnt.Quotas.EnabledChannels,
					EnabledOutputs:  enabledOutputs,
					OnlineChannels:  clnt.Quotas.OnlineChannels,
				},
				ChannelIDs: clnt.ChannelIDs,
			}

			c.JSON(http.StatusOK, res)
			return
		}

		c.JSON(http.StatusOK, p)
	}
}
