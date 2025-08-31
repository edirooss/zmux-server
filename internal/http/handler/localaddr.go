package handler

import (
	"net/http"
	"strconv"

	"github.com/edirooss/zmux-server/internal/service"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type LocalAddrHandler struct {
	log *zap.Logger
	svc *service.LocalAddrLister
}

// NewLocalAddrHandler constructs a LocaladdrHandler instance.
func NewLocalAddrHandler(log *zap.Logger) *LocalAddrHandler {
	return &LocalAddrHandler{
		log: log.Named("localaddr"),
		svc: service.NewLocalAddrLister(service.LocalAddrListerOptions{}), // // Service for reading local addresses
	}
}

func (h *LocalAddrHandler) GetLocalAddrList(c *gin.Context) {
	localAddrs, err := h.svc.GetLocalAddrs(c.Request.Context())
	if err != nil {
		c.Error(err)
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}

	c.Header("X-Total-Count", strconv.Itoa(len(localAddrs))) // RA needs this
	c.JSON(http.StatusOK, localAddrs)
}
