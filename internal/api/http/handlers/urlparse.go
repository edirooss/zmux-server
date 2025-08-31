package handlers

import (
	"net/http"

	"github.com/edirooss/zmux-server/pkg/urlutil"
	"github.com/gin-gonic/gin"
)

type URLParse struct{}

// POST("/api/url/parse", Parse)
func (h *URLParse) Parse(c *gin.Context) {
	var req struct {
		URL string `json:"url"`
	}
	if err := bind(c.Request, &req); err != nil {
		c.Error(err)
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}
	url, err := urlutil.Parse(req.URL)
	if err != nil {
		c.Error(err)
		c.JSON(http.StatusUnprocessableEntity, gin.H{"message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, url)
}

// POST("/api/url/parse/raw", RawParse)
func (h *URLParse) RawParse(c *gin.Context) {
	var req struct {
		URL string `json:"url"`
	}
	if err := bind(c.Request, &req); err != nil {
		c.Error(err)
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}
	url, err := urlutil.RawParse(req.URL)
	if err != nil {
		c.Error(err)
		c.JSON(http.StatusUnprocessableEntity, gin.H{"message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, url)
}
