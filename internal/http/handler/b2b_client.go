package handler

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"

	b2bclient "github.com/edirooss/zmux-server/internal/domain/b2b-client"
	"github.com/edirooss/zmux-server/internal/service"
	"github.com/gin-gonic/gin"
)

type B2BClientHandler struct {
	b2bclntsvc *service.B2BClientService
}

func NewB2BClientHandler(b2bclntsvc *service.B2BClientService) *B2BClientHandler {
	return &B2BClientHandler{b2bclntsvc}
}

func (h *B2BClientHandler) CreateB2BClient(c *gin.Context) {
	var req b2bclient.B2BClientResource
	if err := bind(c.Request, &req); err != nil {
		c.Error(err)
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}

	if view, err := h.b2bclntsvc.Create(c.Request.Context(), &req); err != nil {
		c.Error(err)
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	} else {
		c.Header("Location", fmt.Sprintf("/api/b2b-client/%d", view.ID))
		c.JSON(http.StatusCreated, view)
	}
}

func (h *B2BClientHandler) UpdateB2BClient(c *gin.Context) {
	idStr := c.Param("id")
	b2bClientID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.Error(err)
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}

	var req b2bclient.B2BClientResource
	if err := bind(c.Request, &req); err != nil {
		c.Error(err)
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}

	if view, err := h.b2bclntsvc.Update(c.Request.Context(), b2bClientID, &req); err != nil {
		c.Error(err)

		if errors.Is(err, service.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"message": err.Error()})
		} else if errors.Is(err, service.ErrConflict) {
			c.JSON(http.StatusConflict, gin.H{"message": err.Error()})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		}

		return
	} else {
		c.JSON(http.StatusOK, view)
	}

}

func (h *B2BClientHandler) GetB2BClient(c *gin.Context) {
	idStr := c.Param("id")
	b2bClientID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.Error(err)
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}

	b2bClient, err := h.b2bclntsvc.GetOne(b2bClientID)
	if err != nil {
		c.Error(err)

		if errors.Is(err, service.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"message": err.Error()})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		}

		return
	}

	c.JSON(http.StatusOK, b2bClient)
}

func (h *B2BClientHandler) GetAllB2BClients(c *gin.Context) {
	clients, err := h.b2bclntsvc.GetList()
	if err != nil {
		c.Error(err)
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, clients)
}

func (h *B2BClientHandler) DeleteB2BClient(c *gin.Context) {
	idStr := c.Param("id")
	b2bClientID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.Error(err)
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}

	if err := h.b2bclntsvc.Delete(c.Request.Context(), b2bClientID); err != nil {
		c.Error(err)

		if errors.Is(err, service.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"message": err.Error()})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		}

		return
	}

	c.Status(http.StatusNoContent)
}
