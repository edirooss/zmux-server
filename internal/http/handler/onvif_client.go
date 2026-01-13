package handler

import (
	"net/http"
	"sync"
	"time"

	"github.com/aviravitz/onvif-client/camera"
	"github.com/aviravitz/onvif-client/deviceioservice"
	"github.com/aviravitz/onvif-client/deviceservice"
	"github.com/aviravitz/onvif-client/eventsservice"
	"github.com/aviravitz/onvif-client/imagingservice"
	"github.com/aviravitz/onvif-client/mediaservice"
	"github.com/aviravitz/onvif-client/ptzservice"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type CameraCacheType struct {
	mu    sync.RWMutex
	cache map[string]*camera.Camera
}

var (
	CameraCache = CameraCacheType{
		mu:    sync.RWMutex{},
		cache: make(map[string]*camera.Camera),
	}
)

// Get retrieves a value from the cache.
func (c *CameraCacheType) Get(key string) (*camera.Camera, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	val, exists := c.cache[key]
	if !exists {
		return nil, false
	}
	// Parse the cached string back to camera.Camera if needed
	return val, true
}

// Set stores a value in the cache.
func (c *CameraCacheType) Set(key string, value *camera.Camera) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache[key] = value
}

// DecryptCameraDetails decrypts camera details and caches the result.
func DecryptCameraDetails(encrypted string) (*camera.Camera, error) {
	if decrypted, exists := CameraCache.Get(encrypted); exists {
		return decrypted, nil
	}

	// TODO: implement decryption logic
	// TODO: implement parsing logic
	addr := ""
	port := ""
	user := ""
	password := ""
	cam, err := camera.CreateCamera(addr, port, user, password)
	if err != nil {
		return nil, err
	}
	CameraCache.Set(encrypted, cam)
	return cam, nil
}

// ONVIFClientHandler provides RESTful HTTP handlers for ONVIF client operations.
type ONVIFClientHandler struct {
	log   *zap.Logger
	cache *CameraCacheType
}

// NewONVIFClientHandler constructs an ONVIFClientHandler instance.
func NewONVIFClientHandler(log *zap.Logger) (*ONVIFClientHandler, error) {
	return &ONVIFClientHandler{
		log:   log.Named("onvif_client"),
		cache: &CameraCache,
	}, nil
}

// Device IO handlers

// GetDigitalInputs handles GET /GetDigitalInputs
func (h *ONVIFClientHandler) GetDigitalInputs(c *gin.Context) {
	encryptedCameraDetails := c.Query("encryptedCameraDetails")

	// 1. Validation check
	if encryptedCameraDetails == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "encryptedCameraDetails is required"})
		return
	}

	// 2. Cache retrieval
	cam, exists := h.cache.Get(encryptedCameraDetails)
	if !exists {
		h.log.Warn("camera not found in cache", zap.String("key", encryptedCameraDetails))
		c.JSON(http.StatusNotFound, gin.H{"error": "failed to retrieve camera from cache"})
		return
	}

	h.log.Info("GetDigitalInputs", zap.String("encryptedCameraDetails", encryptedCameraDetails))

	// 3. Service Call
	d, err := deviceioservice.GetDigitalInputs(cam)
	if err != nil {
		h.log.Error("failed to get digital inputs", zap.Error(err))
		// Added error response so client isn't left hanging
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch digital inputs from device"})
		return
	}

	// 4. Proper Gin JSON response
	c.JSON(http.StatusOK, d)
}

// GetRelays handles GET /GetRelays
func (h *ONVIFClientHandler) GetRelays(c *gin.Context) {
	encryptedCameraDetails := c.Query("encryptedCameraDetails")
	if encryptedCameraDetails == "" {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "encryptedCameraDetails is required"})
		return
	}

	cam, exists := h.cache.Get(encryptedCameraDetails)
	if !exists {
		h.log.Warn("camera not found in cache", zap.String("details", encryptedCameraDetails))
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "failed to retrieve camera"})
		return
	}

	h.log.Info("GetRelays", zap.String("encryptedCameraDetails", encryptedCameraDetails))

	relays, err := deviceioservice.GetRelays(cam)
	if err != nil {
		h.log.Error("failed to get relays", zap.Error(err))
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "failed to get relays"})
		return
	}

	// c.JSON automatically sets Content-Type to application/json and encodes the body
	c.JSON(http.StatusOK, relays)
}

// TriggerRelay handles POST /TriggerRelay
func (h *ONVIFClientHandler) TriggerRelay(c *gin.Context) {
	var req struct {
		EncryptedCameraDetails string `json:"encryptedCameraDetails"`
		Token                  string `json:"token"`
		Active                 bool   `json:"active"`
	}

	// Use c.ShouldBindJSON instead of json.NewDecoder(r.Body)
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	cam, exists := h.cache.Get(req.EncryptedCameraDetails)
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "failed to retrieve camera"})
		return
	}

	h.log.Info("TriggerRelay",
		zap.String("encryptedCameraDetails", req.EncryptedCameraDetails),
		zap.String("token", req.Token),
		zap.Bool("active", req.Active),
	)

	err := deviceioservice.TriggerRelay(cam, req.Token, req.Active)
	if err != nil {
		h.log.Error("failed to trigger relay", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to trigger relay"})
		return
	}

	c.Status(http.StatusOK)
}

// GetDeviceInformation handles GET /GetDeviceInformation
func (h *ONVIFClientHandler) GetDeviceInformation(c *gin.Context) {
	encryptedCameraDetails := c.Query("encryptedCameraDetails")
	if encryptedCameraDetails == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "encryptedCameraDetails is required"})
		return
	}

	cam, exists := h.cache.Get(encryptedCameraDetails)
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "failed to retrieve camera"})
		return
	}

	h.log.Info("GetDeviceInformation", zap.String("encryptedCameraDetails", encryptedCameraDetails))

	dev, err := deviceservice.GetDeviceInformation(cam)
	if err != nil {
		h.log.Error("failed to get device information", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get device information"})
		return
	}

	c.JSON(http.StatusOK, dev)
}

// GetSystemDateAndTime handles GET /GetSystemDateAndTime
func (h *ONVIFClientHandler) GetSystemDateAndTime(c *gin.Context) {
	encryptedCameraDetails := c.Query("encryptedCameraDetails")
	if encryptedCameraDetails == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "encryptedCameraDetails is required"})
		return
	}

	cam, exists := h.cache.Get(encryptedCameraDetails)
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "failed to retrieve camera"})
		return
	}

	h.log.Info("GetSystemDateAndTime", zap.String("encryptedCameraDetails", encryptedCameraDetails))

	result, err := deviceservice.GetSystemDateAndTime(cam)
	if err != nil {
		h.log.Error("failed to get system date and time", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get system date and time"})
		return
	}

	c.JSON(http.StatusOK, result)
}

// GetNetworkInterfaces handles GET /GetNetworkInterfaces
func (h *ONVIFClientHandler) GetNetworkInterfaces(c *gin.Context) {
	encryptedCameraDetails := c.Query("encryptedCameraDetails")
	if encryptedCameraDetails == "" {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "encryptedCameraDetails is required"})
		return
	}
	cam, exists := h.cache.Get(encryptedCameraDetails)
	if !exists {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "failed to retrieve camera"})
		return
	}
	h.log.Info("GetNetworkInterfaces", zap.String("encryptedCameraDetails", encryptedCameraDetails))

	result, err := deviceservice.GetNetworkInterfaces(cam)
	if err != nil {
		h.log.Error("failed to get network interfaces", zap.Error(err))
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "failed to get network interfaces"})
		return
	}

	c.JSON(http.StatusOK, result)
}

// GetUsers handles GET /GetUsers
func (h *ONVIFClientHandler) GetUsers(c *gin.Context) {
	encryptedCameraDetails := c.Query("encryptedCameraDetails")
	if encryptedCameraDetails == "" {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "encryptedCameraDetails is required"})
		return
	}
	cam, exists := h.cache.Get(encryptedCameraDetails)
	if !exists {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "failed to retrieve camera"})
		return
	}
	h.log.Info("GetUsers", zap.String("encryptedCameraDetails", encryptedCameraDetails))

	result, err := deviceservice.GetUsers(cam)
	if err != nil {
		h.log.Error("failed to get users", zap.Error(err))
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "failed to get users"})
		return
	}

	c.JSON(http.StatusOK, result)
}

// GetDNS handles GET /GetDNS
func (h *ONVIFClientHandler) GetDNS(c *gin.Context) {
	encryptedCameraDetails := c.Query("encryptedCameraDetails")
	if encryptedCameraDetails == "" {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "encryptedCameraDetails is required"})
		return
	}
	cam, exists := h.cache.Get(encryptedCameraDetails)
	if !exists {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "failed to retrieve camera"})
		return
	}
	h.log.Info("GetDNS", zap.String("encryptedCameraDetails", encryptedCameraDetails))

	result, err := deviceservice.GetDNS(cam)
	if err != nil {
		h.log.Error("failed to get DNS", zap.Error(err))
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "failed to get DNS"})
		return
	}

	c.JSON(http.StatusOK, result)
}

// GetScopes handles GET /GetScopes
func (h *ONVIFClientHandler) GetScopes(c *gin.Context) {
	encryptedCameraDetails := c.Query("encryptedCameraDetails")
	if encryptedCameraDetails == "" {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "encryptedCameraDetails is required"})
		return
	}
	cam, exists := h.cache.Get(encryptedCameraDetails)
	if !exists {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "failed to retrieve camera"})
		return
	}
	h.log.Info("GetScopes", zap.String("encryptedCameraDetails", encryptedCameraDetails))

	result, err := deviceservice.GetScopes(cam)
	if err != nil {
		h.log.Error("failed to get scopes", zap.Error(err))
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "failed to get scopes"})
		return
	}

	c.JSON(http.StatusOK, result)
}

// GetNTP handles GET /GetNTP
func (h *ONVIFClientHandler) GetNTP(c *gin.Context) {
	encryptedCameraDetails := c.Query("encryptedCameraDetails")
	if encryptedCameraDetails == "" {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "encryptedCameraDetails is required"})
		return
	}
	cam, exists := h.cache.Get(encryptedCameraDetails)
	if !exists {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "failed to retrieve camera"})
		return
	}
	h.log.Info("GetNTP", zap.String("encryptedCameraDetails", encryptedCameraDetails))

	result, err := deviceservice.GetNTP(cam)
	if err != nil {
		h.log.Error("failed to get NTP", zap.Error(err))
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "failed to get NTP"})
		return
	}

	c.JSON(http.StatusOK, result)
}

// RebootCamera handles POST /RebootCamera
func (h *ONVIFClientHandler) RebootCamera(c *gin.Context) {
	var req struct {
		EncryptedCameraDetails string `json:"encryptedCameraDetails"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	cam, exists := h.cache.Get(req.EncryptedCameraDetails)
	if !exists {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "failed to retrieve camera"})
		return
	}
	h.log.Info("RebootCamera", zap.String("encryptedCameraDetails", req.EncryptedCameraDetails))

	err := deviceservice.RebootCamera(cam)
	if err != nil {
		h.log.Error("failed to reboot camera", zap.Error(err))
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "failed to reboot camera"})
		return
	}
	c.Status(http.StatusOK)
}

// SetNTP handles POST /SetNTP
func (h *ONVIFClientHandler) SetNTP(c *gin.Context) {
	var req struct {
		EncryptedCameraDetails string `json:"encryptedCameraDetails"`
		FromDHCP               bool   `json:"fromDHCP"`
		Server                 string `json:"server"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	cam, exists := h.cache.Get(req.EncryptedCameraDetails)
	if !exists {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "failed to retrieve camera"})
		return
	}
	h.log.Info("SetNTP", zap.String("encryptedCameraDetails", req.EncryptedCameraDetails), zap.Bool("fromDHCP", req.FromDHCP), zap.String("server", req.Server))

	err := deviceservice.SetNTP(cam, req.FromDHCP, req.Server)
	if err != nil {
		h.log.Error("failed to set NTP", zap.Error(err))
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "failed to set NTP"})
		return
	}
	c.Status(http.StatusOK)
}

// GetSystemLog handles GET /GetSystemLog
func (h *ONVIFClientHandler) GetSystemLog(c *gin.Context) {
	encryptedCameraDetails := c.Query("encryptedCameraDetails")
	logType := c.Query("logType")
	if encryptedCameraDetails == "" || logType == "" {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "encryptedCameraDetails and logType are required"})
		return
	}
	cam, exists := h.cache.Get(encryptedCameraDetails)
	if !exists {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "failed to retrieve camera"})
		return
	}
	h.log.Info("GetSystemLog", zap.String("encryptedCameraDetails", encryptedCameraDetails), zap.String("logType", logType))

	result, err := deviceservice.GetSystemLog(cam, logType)
	if err != nil {
		h.log.Error("failed to get system log", zap.Error(err))
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "failed to get system log"})
		return
	}

	c.JSON(http.StatusOK, result)
}

// StartSubscription handles POST /StartSubscription
func (h *ONVIFClientHandler) StartSubscription(c *gin.Context) {
	var req struct {
		EncryptedCameraDetails string `json:"encryptedCameraDetails"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	cam, exists := h.cache.Get(req.EncryptedCameraDetails)
	if !exists {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "failed to retrieve camera"})
		return
	}
	h.log.Info("StartSubscription", zap.String("encryptedCameraDetails", req.EncryptedCameraDetails))

	subscriptionUrl, err := eventsservice.StartSubscription(cam)
	if err != nil {
		h.log.Error("failed to start subscription", zap.Error(err))
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "failed to start subscription"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"subscriptionUrl": subscriptionUrl})
}

// FetchEvents handles GET /FetchEvents
func (h *ONVIFClientHandler) FetchEvents(c *gin.Context) {
	encryptedCameraDetails := c.Query("encryptedCameraDetails")
	subscriptionUrl := c.Query("subscriptionUrl")
	if encryptedCameraDetails == "" || subscriptionUrl == "" {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "encryptedCameraDetails and subscriptionUrl are required"})
		return
	}
	cam, exists := h.cache.Get(encryptedCameraDetails)
	if !exists {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "failed to retrieve camera"})
		return
	}
	h.log.Info("FetchEvents", zap.String("encryptedCameraDetails", encryptedCameraDetails), zap.String("subscriptionUrl", subscriptionUrl))

	events, err := eventsservice.FetchEvents(cam, subscriptionUrl)
	if err != nil {
		h.log.Error("failed to fetch events", zap.Error(err))
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch events"})
		return
	}

	c.JSON(http.StatusOK, events)
}

// RenewSubscription handles POST /RenewSubscription
// RenewSubscription handles POST /RenewSubscription
func (h *ONVIFClientHandler) RenewSubscription(c *gin.Context) {
	var req struct {
		EncryptedCameraDetails string `json:"encryptedCameraDetails"`
		SubscriptionUrl        string `json:"subscriptionUrl"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	cam, exists := h.cache.Get(req.EncryptedCameraDetails)
	if !exists {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "failed to retrieve camera"})
		return
	}
	h.log.Info("RenewSubscription", zap.String("encryptedCameraDetails", req.EncryptedCameraDetails), zap.String("subscriptionUrl", req.SubscriptionUrl))

	err := eventsservice.RenewSubscription(cam, req.SubscriptionUrl)
	if err != nil {
		h.log.Error("failed to renew subscription", zap.Error(err))
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "failed to renew subscription"})
		return
	}
	c.Status(http.StatusOK)
}

// Imaging handlers

// GetImagingSettings handles GET /GetImagingSettings
func (h *ONVIFClientHandler) GetImagingSettings(c *gin.Context) {
	encryptedCameraDetails := c.Query("encryptedCameraDetails")
	videoSourceToken := c.Query("videoSourceToken")
	if encryptedCameraDetails == "" || videoSourceToken == "" {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "encryptedCameraDetails and videoSourceToken are required"})
		return
	}
	cam, exists := h.cache.Get(encryptedCameraDetails)
	if !exists {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "failed to retrieve camera"})
		return
	}
	h.log.Info("GetImagingSettings", zap.String("encryptedCameraDetails", encryptedCameraDetails), zap.String("videoSourceToken", videoSourceToken))

	result, err := imagingservice.GetImagingSettings(cam, videoSourceToken)
	if err != nil {
		h.log.Error("failed to get imaging settings", zap.Error(err))
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "failed to get imaging settings"})
		return
	}

	c.JSON(http.StatusOK, result)
}

// GetImagingOptions handles GET /GetImagingOptions
func (h *ONVIFClientHandler) GetImagingOptions(c *gin.Context) {
	encryptedCameraDetails := c.Query("encryptedCameraDetails")
	videoSourceToken := c.Query("videoSourceToken")
	if encryptedCameraDetails == "" || videoSourceToken == "" {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "encryptedCameraDetails and videoSourceToken are required"})
		return
	}
	cam, exists := h.cache.Get(encryptedCameraDetails)
	if !exists {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "failed to retrieve camera"})
		return
	}
	h.log.Info("GetImagingOptions", zap.String("encryptedCameraDetails", encryptedCameraDetails), zap.String("videoSourceToken", videoSourceToken))

	result, err := imagingservice.GetImagingOptions(cam, videoSourceToken)
	if err != nil {
		h.log.Error("failed to get imaging options", zap.Error(err))
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "failed to get imaging options"})
		return
	}

	c.JSON(http.StatusOK, result)
}

// GetImagingStatus handles GET /GetImagingStatus
func (h *ONVIFClientHandler) GetImagingStatus(c *gin.Context) {
	encryptedCameraDetails := c.Query("encryptedCameraDetails")
	videoSourceToken := c.Query("videoSourceToken")
	if encryptedCameraDetails == "" || videoSourceToken == "" {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "encryptedCameraDetails and videoSourceToken are required"})
		return
	}
	cam, exists := h.cache.Get(encryptedCameraDetails)
	if !exists {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "failed to retrieve camera"})
		return
	}
	h.log.Info("GetImagingStatus", zap.String("encryptedCameraDetails", encryptedCameraDetails), zap.String("videoSourceToken", videoSourceToken))

	result, err := imagingservice.GetImagingStatus(cam, videoSourceToken)
	if err != nil {
		h.log.Error("failed to get imaging status", zap.Error(err))
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "failed to get imaging status"})
		return
	}

	c.JSON(http.StatusOK, result)
}

// IsManualFocus handles GET /IsManualFocus
func (h *ONVIFClientHandler) IsManualFocus(c *gin.Context) {
	encryptedCameraDetails := c.Query("encryptedCameraDetails")
	token := c.Query("token")
	if encryptedCameraDetails == "" || token == "" {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "encryptedCameraDetails and token are required"})
		return
	}
	cam, exists := h.cache.Get(encryptedCameraDetails)
	if !exists {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "failed to retrieve camera"})
		return
	}
	h.log.Info("IsManualFocus", zap.String("encryptedCameraDetails", encryptedCameraDetails), zap.String("token", token))

	result := imagingservice.IsManualFocus(cam, token)
	c.JSON(http.StatusOK, gin.H{"isManualFocus": result})
}

// SetImagingSettings handles POST /SetImagingSettings
func (h *ONVIFClientHandler) SetImagingSettings(c *gin.Context) {
	var req struct {
		EncryptedCameraDetails string  `json:"encryptedCameraDetails"`
		VideoSourceToken       string  `json:"videoSourceToken"`
		NewBright              float64 `json:"newBright"`
		NewContrast            float64 `json:"newContrast"`
		NewSat                 float64 `json:"newSat"`
		NewSharpness           float64 `json:"newSharpness"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	cam, exists := h.cache.Get(req.EncryptedCameraDetails)
	if !exists {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "failed to retrieve camera"})
		return
	}
	h.log.Info("SetImagingSettings", zap.String("encryptedCameraDetails", req.EncryptedCameraDetails), zap.String("videoSourceToken", req.VideoSourceToken))

	err := imagingservice.SetImagingSettings(cam, req.VideoSourceToken, req.NewBright, req.NewContrast, req.NewSat, req.NewSharpness)
	if err != nil {
		h.log.Error("failed to set imaging settings", zap.Error(err))
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "failed to set imaging settings"})
		return
	}
	c.Status(http.StatusOK)
}

// MoveFocusAbsolute handles POST /MoveFocusAbsolute
func (h *ONVIFClientHandler) MoveFocusAbsolute(c *gin.Context) {
	var req struct {
		EncryptedCameraDetails string  `json:"encryptedCameraDetails"`
		VideoSourceToken       string  `json:"videoSourceToken"`
		Position               float64 `json:"position"`
		Speed                  float64 `json:"speed"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	cam, exists := h.cache.Get(req.EncryptedCameraDetails)
	if !exists {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "failed to retrieve camera"})
		return
	}
	h.log.Info("MoveFocusAbsolute", zap.String("encryptedCameraDetails", req.EncryptedCameraDetails), zap.String("videoSourceToken", req.VideoSourceToken))

	err := imagingservice.MoveFocusAbsolute(cam, req.VideoSourceToken, req.Position, req.Speed)
	if err != nil {
		h.log.Error("failed to move focus absolute", zap.Error(err))
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "failed to move focus absolute"})
		return
	}
	c.Status(http.StatusOK)
}

// MoveFocusRelative handles POST /MoveFocusRelative
func (h *ONVIFClientHandler) MoveFocusRelative(c *gin.Context) {
	var req struct {
		EncryptedCameraDetails string  `json:"encryptedCameraDetails"`
		VideoSourceToken       string  `json:"videoSourceToken"`
		Distance               float64 `json:"distance"`
		Speed                  float64 `json:"speed"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	cam, exists := h.cache.Get(req.EncryptedCameraDetails)
	if !exists {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "failed to retrieve camera"})
		return
	}
	h.log.Info("MoveFocusRelative", zap.String("encryptedCameraDetails", req.EncryptedCameraDetails), zap.String("videoSourceToken", req.VideoSourceToken))

	err := imagingservice.MoveFocusRelative(cam, req.VideoSourceToken, req.Distance, req.Speed)
	if err != nil {
		h.log.Error("failed to move focus relative", zap.Error(err))
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "failed to move focus relative"})
		return
	}
	c.Status(http.StatusOK)
}

// StartFocusMove handles POST /StartFocusMove
func (h *ONVIFClientHandler) StartFocusMove(c *gin.Context) {
	var req struct {
		EncryptedCameraDetails string  `json:"encryptedCameraDetails"`
		VideoSourceToken       string  `json:"videoSourceToken"`
		Speed                  float64 `json:"speed"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	cam, exists := h.cache.Get(req.EncryptedCameraDetails)
	if !exists {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "failed to retrieve camera"})
		return
	}
	h.log.Info("StartFocusMove", zap.String("encryptedCameraDetails", req.EncryptedCameraDetails), zap.String("videoSourceToken", req.VideoSourceToken))

	err := imagingservice.StartFocusMove(cam, req.VideoSourceToken, req.Speed)
	if err != nil {
		h.log.Error("failed to start focus move", zap.Error(err))
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "failed to start focus move"})
		return
	}
	c.Status(http.StatusOK)
}

// StopFocus handles POST /StopFocus
func (h *ONVIFClientHandler) StopFocus(c *gin.Context) {
	var req struct {
		EncryptedCameraDetails string `json:"encryptedCameraDetails"`
		VideoSourceToken       string `json:"videoSourceToken"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	cam, exists := h.cache.Get(req.EncryptedCameraDetails)
	if !exists {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "failed to retrieve camera"})
		return
	}
	h.log.Info("StopFocus", zap.String("encryptedCameraDetails", req.EncryptedCameraDetails), zap.String("videoSourceToken", req.VideoSourceToken))

	err := imagingservice.StopFocus(cam, req.VideoSourceToken)
	if err != nil {
		h.log.Error("failed to stop focus", zap.Error(err))
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "failed to stop focus"})
		return
	}
	c.Status(http.StatusOK)
}

// SetFocusMode handles POST /SetFocusMode
func (h *ONVIFClientHandler) SetFocusMode(c *gin.Context) {
	var req struct {
		EncryptedCameraDetails string `json:"encryptedCameraDetails"`
		VideoSourceToken       string `json:"videoSourceToken"`
		Mode                   string `json:"mode"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	cam, exists := h.cache.Get(req.EncryptedCameraDetails)
	if !exists {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "failed to retrieve camera"})
		return
	}
	h.log.Info("SetFocusMode", zap.String("encryptedCameraDetails", req.EncryptedCameraDetails), zap.String("videoSourceToken", req.VideoSourceToken), zap.String("mode", req.Mode))

	err := imagingservice.SetFocusMode(cam, req.VideoSourceToken, req.Mode)
	if err != nil {
		h.log.Error("failed to set focus mode", zap.Error(err))
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "failed to set focus mode"})
		return
	}
	c.Status(http.StatusOK)
}

// SetIrCutFilter handles POST /SetIrCutFilter
func (h *ONVIFClientHandler) SetIrCutFilter(c *gin.Context) {
	var req struct {
		EncryptedCameraDetails string `json:"encryptedCameraDetails"`
		VideoSourceToken       string `json:"videoSourceToken"`
		Mode                   string `json:"mode"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	cam, exists := h.cache.Get(req.EncryptedCameraDetails)
	if !exists {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "failed to retrieve camera"})
		return
	}
	h.log.Info("SetIrCutFilter", zap.String("encryptedCameraDetails", req.EncryptedCameraDetails), zap.String("videoSourceToken", req.VideoSourceToken), zap.String("mode", req.Mode))

	err := imagingservice.SetIrCutFilter(cam, req.VideoSourceToken, req.Mode)
	if err != nil {
		h.log.Error("failed to set IR cut filter", zap.Error(err))
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "failed to set IR cut filter"})
		return
	}
	c.Status(http.StatusOK)
}

// SetBacklightCompensation handles POST /SetBacklightCompensation
func (h *ONVIFClientHandler) SetBacklightCompensation(c *gin.Context) {
	var req struct {
		EncryptedCameraDetails string  `json:"encryptedCameraDetails"`
		VideoSourceToken       string  `json:"videoSourceToken"`
		Mode                   string  `json:"mode"`
		Level                  float64 `json:"level"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	cam, exists := h.cache.Get(req.EncryptedCameraDetails)
	if !exists {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "failed to retrieve camera"})
		return
	}
	h.log.Info("SetBacklightCompensation", zap.String("encryptedCameraDetails", req.EncryptedCameraDetails), zap.String("videoSourceToken", req.VideoSourceToken), zap.String("mode", req.Mode))

	err := imagingservice.SetBacklightCompensation(cam, req.VideoSourceToken, req.Mode, req.Level)
	if err != nil {
		h.log.Error("failed to set backlight compensation", zap.Error(err))
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "failed to set backlight compensation"})
		return
	}
	c.Status(http.StatusOK)
}

// SetWideDynamicRange handles POST /SetWideDynamicRange
func (h *ONVIFClientHandler) SetWideDynamicRange(c *gin.Context) {
	var req struct {
		EncryptedCameraDetails string  `json:"encryptedCameraDetails"`
		VideoSourceToken       string  `json:"videoSourceToken"`
		Mode                   string  `json:"mode"`
		Level                  float64 `json:"level"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	cam, exists := h.cache.Get(req.EncryptedCameraDetails)
	if !exists {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "failed to retrieve camera"})
		return
	}
	h.log.Info("SetWideDynamicRange", zap.String("encryptedCameraDetails", req.EncryptedCameraDetails), zap.String("videoSourceToken", req.VideoSourceToken), zap.String("mode", req.Mode))

	err := imagingservice.SetWideDynamicRange(cam, req.VideoSourceToken, req.Mode, req.Level)
	if err != nil {
		h.log.Error("failed to set wide dynamic range", zap.Error(err))
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "failed to set wide dynamic range"})
		return
	}
	c.Status(http.StatusOK)
}

// SetWhiteBalance handles POST /SetWhiteBalance
func (h *ONVIFClientHandler) SetWhiteBalance(c *gin.Context) {
	var req struct {
		EncryptedCameraDetails string  `json:"encryptedCameraDetails"`
		VideoSourceToken       string  `json:"videoSourceToken"`
		Mode                   string  `json:"mode"`
		CrGain                 float64 `json:"crGain"`
		CbGain                 float64 `json:"cbGain"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	cam, exists := h.cache.Get(req.EncryptedCameraDetails)
	if !exists {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "failed to retrieve camera"})
		return
	}
	h.log.Info("SetWhiteBalance", zap.String("encryptedCameraDetails", req.EncryptedCameraDetails), zap.String("videoSourceToken", req.VideoSourceToken), zap.String("mode", req.Mode))

	err := imagingservice.SetWhiteBalance(cam, req.VideoSourceToken, req.Mode, req.CrGain, req.CbGain)
	if err != nil {
		h.log.Error("failed to set white balance", zap.Error(err))
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "failed to set white balance"})
		return
	}
	c.Status(http.StatusOK)
}

// SetExposureMode handles POST /SetExposureMode
func (h *ONVIFClientHandler) SetExposureMode(c *gin.Context) {
	var req struct {
		EncryptedCameraDetails string `json:"encryptedCameraDetails"`
		VideoSourceToken       string `json:"videoSourceToken"`
		Mode                   string `json:"mode"`
		Priority               string `json:"priority"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	cam, exists := h.cache.Get(req.EncryptedCameraDetails)
	if !exists {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "failed to retrieve camera"})
		return
	}
	h.log.Info("SetExposureMode", zap.String("encryptedCameraDetails", req.EncryptedCameraDetails), zap.String("videoSourceToken", req.VideoSourceToken), zap.String("mode", req.Mode))

	err := imagingservice.SetExposureMode(cam, req.VideoSourceToken, req.Mode, req.Priority)
	if err != nil {
		h.log.Error("failed to set exposure mode", zap.Error(err))
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "failed to set exposure mode"})
		return
	}
	c.Status(http.StatusOK)
}

// SetManualExposure handles POST /SetManualExposure
func (h *ONVIFClientHandler) SetManualExposure(c *gin.Context) {
	var req struct {
		EncryptedCameraDetails string  `json:"encryptedCameraDetails"`
		VideoSourceToken       string  `json:"videoSourceToken"`
		ExpTime                float64 `json:"expTime"`
		Gain                   float64 `json:"gain"`
		Iris                   float64 `json:"iris"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	cam, exists := h.cache.Get(req.EncryptedCameraDetails)
	if !exists {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "failed to retrieve camera"})
		return
	}
	h.log.Info("SetManualExposure", zap.String("encryptedCameraDetails", req.EncryptedCameraDetails), zap.String("videoSourceToken", req.VideoSourceToken))

	err := imagingservice.SetManualExposure(cam, req.VideoSourceToken, req.ExpTime, req.Gain, req.Iris)
	if err != nil {
		h.log.Error("failed to set manual exposure", zap.Error(err))
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "failed to set manual exposure"})
		return
	}
	c.Status(http.StatusOK)
}

// SetExposureLimits handles POST /SetExposureLimits
func (h *ONVIFClientHandler) SetExposureLimits(c *gin.Context) {
	var req struct {
		EncryptedCameraDetails string  `json:"encryptedCameraDetails"`
		VideoSourceToken       string  `json:"videoSourceToken"`
		MinTime                float64 `json:"minTime"`
		MaxTime                float64 `json:"maxTime"`
		MinGain                float64 `json:"minGain"`
		MaxGain                float64 `json:"maxGain"`
		MinIris                float64 `json:"minIris"`
		MaxIris                float64 `json:"maxIris"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	cam, exists := h.cache.Get(req.EncryptedCameraDetails)
	if !exists {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "failed to retrieve camera"})
		return
	}
	h.log.Info("SetExposureLimits", zap.String("encryptedCameraDetails", req.EncryptedCameraDetails), zap.String("videoSourceToken", req.VideoSourceToken))

	err := imagingservice.SetExposureLimits(cam, req.VideoSourceToken, req.MinTime, req.MaxTime, req.MinGain, req.MaxGain, req.MinIris, req.MaxIris)
	if err != nil {
		h.log.Error("failed to set exposure limits", zap.Error(err))
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "failed to set exposure limits"})
		return
	}
	c.Status(http.StatusOK)
}

// Media handlers
// GetProfileToken handles GET /GetProfileToken
func (h *ONVIFClientHandler) GetProfileToken(c *gin.Context) {
	encryptedCameraDetails := c.Query("encryptedCameraDetails")
	if encryptedCameraDetails == "" {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "encryptedCameraDetails is required"})
		return
	}
	cam, exists := h.cache.Get(encryptedCameraDetails)
	if !exists {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "failed to retrieve camera"})
		return
	}
	h.log.Info("GetProfileToken", zap.String("encryptedCameraDetails", encryptedCameraDetails))

	token := mediaservice.GetProfileToken(cam)
	c.JSON(http.StatusOK, gin.H{"token": token})
}

// GetSensorToken handles GET /GetSensorToken
func (h *ONVIFClientHandler) GetSensorToken(c *gin.Context) {
	encryptedCameraDetails := c.Query("encryptedCameraDetails")
	if encryptedCameraDetails == "" {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "encryptedCameraDetails is required"})
		return
	}
	cam, exists := h.cache.Get(encryptedCameraDetails)
	if !exists {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "failed to retrieve camera"})
		return
	}
	h.log.Info("GetSensorToken", zap.String("encryptedCameraDetails", encryptedCameraDetails))

	token := mediaservice.GetSensorToken(cam)
	c.JSON(http.StatusOK, gin.H{"token": token})
}

// GetDeviceProfiles handles GET /GetDeviceProfiles
func (h *ONVIFClientHandler) GetDeviceProfiles(c *gin.Context) {
	encryptedCameraDetails := c.Query("encryptedCameraDetails")
	if encryptedCameraDetails == "" {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "encryptedCameraDetails is required"})
		return
	}
	cam, exists := h.cache.Get(encryptedCameraDetails)
	if !exists {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "failed to retrieve camera"})
		return
	}
	h.log.Info("GetDeviceProfiles", zap.String("encryptedCameraDetails", encryptedCameraDetails))

	profiles, err := mediaservice.GetDeviceProfiles(cam)
	if err != nil {
		h.log.Error("failed to get device profiles", zap.Error(err))
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "failed to get device profiles"})
		return
	}

	c.JSON(http.StatusOK, profiles)
}

// GetStreamUri handles GET /GetStreamUri
func (h *ONVIFClientHandler) GetStreamUri(c *gin.Context) {
	encryptedCameraDetails := c.Query("encryptedCameraDetails")
	token := c.Query("token")
	if encryptedCameraDetails == "" || token == "" {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "encryptedCameraDetails and token are required"})
		return
	}
	cam, exists := h.cache.Get(encryptedCameraDetails)
	if !exists {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "failed to retrieve camera"})
		return
	}
	h.log.Info("GetStreamUri", zap.String("encryptedCameraDetails", encryptedCameraDetails), zap.String("token", token))

	uri, err := mediaservice.GetStreamUri(cam, token)
	if err != nil {
		h.log.Error("failed to get stream URI", zap.Error(err))
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "failed to get stream URI"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"uri": uri})
}

// GetSnapshotUri handles GET /GetSnapshotUri
func (h *ONVIFClientHandler) GetSnapshotUri(c *gin.Context) {
	encryptedCameraDetails := c.Query("encryptedCameraDetails")
	token := c.Query("token")
	if encryptedCameraDetails == "" || token == "" {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "encryptedCameraDetails and token are required"})
		return
	}
	cam, exists := h.cache.Get(encryptedCameraDetails)
	if !exists {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "failed to retrieve camera"})
		return
	}
	h.log.Info("GetSnapshotUri", zap.String("encryptedCameraDetails", encryptedCameraDetails), zap.String("token", token))

	uri, err := mediaservice.GetSnapshotUri(cam, token)
	if err != nil {
		h.log.Error("failed to get snapshot URI", zap.Error(err))
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "failed to get snapshot URI"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"uri": uri})
}

// GetVideoEncoderConfigurations handles GET /GetVideoEncoderConfigurations
func (h *ONVIFClientHandler) GetVideoEncoderConfigurations(c *gin.Context) {
	encryptedCameraDetails := c.Query("encryptedCameraDetails")
	if encryptedCameraDetails == "" {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "encryptedCameraDetails is required"})
		return
	}
	cam, exists := h.cache.Get(encryptedCameraDetails)
	if !exists {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "failed to retrieve camera"})
		return
	}
	h.log.Info("GetVideoEncoderConfigurations", zap.String("encryptedCameraDetails", encryptedCameraDetails))

	configs, err := mediaservice.GetVideoEncoderConfigurations(cam)
	if err != nil {
		h.log.Error("failed to get video encoder configurations", zap.Error(err))
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "failed to get video encoder configurations"})
		return
	}

	c.JSON(http.StatusOK, configs)
}

// GetVideoEncoderConfiguration handles GET /GetVideoEncoderConfiguration
func (h *ONVIFClientHandler) GetVideoEncoderConfiguration(c *gin.Context) {
	encryptedCameraDetails := c.Query("encryptedCameraDetails")
	token := c.Query("token")
	if encryptedCameraDetails == "" || token == "" {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "encryptedCameraDetails and token are required"})
		return
	}
	cam, exists := h.cache.Get(encryptedCameraDetails)
	if !exists {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "failed to retrieve camera"})
		return
	}
	h.log.Info("GetVideoEncoderConfiguration", zap.String("encryptedCameraDetails", encryptedCameraDetails), zap.String("token", token))

	config, err := mediaservice.GetVideoEncoderConfiguration(cam, token)
	if err != nil {
		h.log.Error("failed to get video encoder configuration", zap.Error(err))
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "failed to get video encoder configuration"})
		return
	}

	c.JSON(http.StatusOK, config)
}

// GetOSDs handles GET /GetOSDs
func (h *ONVIFClientHandler) GetOSDs(c *gin.Context) {
	encryptedCameraDetails := c.Query("encryptedCameraDetails")
	token := c.Query("token")
	if encryptedCameraDetails == "" || token == "" {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "encryptedCameraDetails and token are required"})
		return
	}
	cam, exists := h.cache.Get(encryptedCameraDetails)
	if !exists {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "failed to retrieve camera"})
		return
	}
	h.log.Info("GetOSDs", zap.String("encryptedCameraDetails", encryptedCameraDetails), zap.String("token", token))

	osds, err := mediaservice.GetOSDs(cam, token)
	if err != nil {
		h.log.Error("failed to get OSDs", zap.Error(err))
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "failed to get OSDs"})
		return
	}

	c.JSON(http.StatusOK, osds)
}

// GetOSD handles GET /GetOSD
func (h *ONVIFClientHandler) SetOSDText(c *gin.Context) {
	var req struct {
		EncryptedCameraDetails string `json:"encryptedCameraDetails"`
		OsdToken               string `json:"osdToken"`
		VideoSourceToken       string `json:"videoSourceToken"`
		NewText                string `json:"newText"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	cam, exists := h.cache.Get(req.EncryptedCameraDetails)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve camera"})
		return
	}
	h.log.Info("SetOSDText", zap.String("encryptedCameraDetails", req.EncryptedCameraDetails), zap.String("osdToken", req.OsdToken), zap.String("newText", req.NewText))

	err := mediaservice.SetOSDText(cam, req.OsdToken, req.VideoSourceToken, req.NewText)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to set OSD text"})
		return
	}
	c.Status(http.StatusOK)
}

// DeleteOSD handles POST /DeleteOSD
func (h *ONVIFClientHandler) DeleteOSD(c *gin.Context) {
	var req struct {
		EncryptedCameraDetails string `json:"encryptedCameraDetails"`
		OsdToken               string `json:"osdToken"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	cam, exists := h.cache.Get(req.EncryptedCameraDetails)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve camera"})
		return
	}
	h.log.Info("DeleteOSD", zap.String("encryptedCameraDetails", req.EncryptedCameraDetails), zap.String("osdToken", req.OsdToken))

	err := mediaservice.DeleteOSD(cam, req.OsdToken)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete OSD"})
		return
	}
	c.Status(http.StatusOK)
}
func (h *ONVIFClientHandler) GetOSD(c *gin.Context) {
	encryptedCameraDetails := c.Query("encryptedCameraDetails")
	token := c.Query("token")
	if encryptedCameraDetails == "" || token == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "encryptedCameraDetails and token are required"})
		return
	}
	cam, exists := h.cache.Get(encryptedCameraDetails)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve camera"})
		return
	}
	h.log.Info("GetOSD", zap.String("encryptedCameraDetails", encryptedCameraDetails), zap.String("token", token))

	osd, err := mediaservice.GetOSDs(cam, token)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get OSD"})
		return
	}

	c.JSON(http.StatusOK, osd)
}

// GetVideoSources handles GET /GetVideoSources
// GetVideoSources handles GET /GetVideoSources
func (h *ONVIFClientHandler) GetVideoSources(c *gin.Context) {
	encryptedCameraDetails := c.Query("encryptedCameraDetails")
	if encryptedCameraDetails == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "encryptedCameraDetails is required"})
		return
	}

	cam, exists := h.cache.Get(encryptedCameraDetails)
	if !exists {
		h.log.Error("camera not found in cache", zap.String("details", encryptedCameraDetails))
		c.JSON(http.StatusNotFound, gin.H{"error": "failed to retrieve camera"})
		return
	}

	h.log.Info("GetVideoSources", zap.String("encryptedCameraDetails", encryptedCameraDetails))

	sources, err := mediaservice.GetVideoSources(cam)
	if err != nil {
		h.log.Error("failed to get video sources", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get video sources"})
		return
	}

	c.JSON(http.StatusOK, sources)
}

// GetAudioEncoders handles GET /GetAudioEncoders
func (h *ONVIFClientHandler) GetAudioEncoders(c *gin.Context) {
	encryptedCameraDetails := c.Query("encryptedCameraDetails")
	if encryptedCameraDetails == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "encryptedCameraDetails is required"})
		return
	}

	cam, exists := h.cache.Get(encryptedCameraDetails)
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "failed to retrieve camera"})
		return
	}

	h.log.Info("GetAudioEncoders", zap.String("encryptedCameraDetails", encryptedCameraDetails))

	encoders, err := mediaservice.GetAudioEncoders(cam)
	if err != nil {
		h.log.Error("failed to get audio encoders", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get audio encoders"})
		return
	}

	c.JSON(http.StatusOK, encoders)
}

// GetVideoOptions handles GET /GetVideoOptions
func (h *ONVIFClientHandler) GetVideoOptions(c *gin.Context) {
	encryptedCameraDetails := c.Query("encryptedCameraDetails")
	profileToken := c.Query("profileToken")
	if encryptedCameraDetails == "" || profileToken == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "encryptedCameraDetails and profileToken are required"})
		return
	}

	cam, exists := h.cache.Get(encryptedCameraDetails)
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "failed to retrieve camera"})
		return
	}

	h.log.Info("GetVideoOptions",
		zap.String("encryptedCameraDetails", encryptedCameraDetails),
		zap.String("profileToken", profileToken),
	)

	options, err := mediaservice.GetVideoOptions(cam, profileToken)
	if err != nil {
		h.log.Error("failed to get video options", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get video options"})
		return
	}

	c.JSON(http.StatusOK, options)
}

// GetOSDTokenByText handles GET /GetOSDTokenByText
func (h *ONVIFClientHandler) GetOSDTokenByText(c *gin.Context) {
	encryptedCameraDetails := c.Query("encryptedCameraDetails")
	configToken := c.Query("configToken")
	targetText := c.Query("targetText")

	if encryptedCameraDetails == "" || configToken == "" || targetText == "" {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "encryptedCameraDetails, configToken, and targetText are required"})
		return
	}

	cam, exists := h.cache.Get(encryptedCameraDetails)
	if !exists {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "failed to retrieve camera"})
		return
	}

	h.log.Info("GetOSDTokenByText", zap.String("encryptedCameraDetails", encryptedCameraDetails))

	token, err := mediaservice.GetOSDTokenByText(cam, configToken, targetText)
	if err != nil {
		h.log.Error("failed to get OSD token", zap.Error(err))
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "failed to get OSD token by text"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"token": token})
}

// CreateOSD handles POST /CreateOSD
func (h *ONVIFClientHandler) CreateOSD(c *gin.Context) {
	var req struct {
		EncryptedCameraDetails string  `json:"encryptedCameraDetails" binding:"required"`
		VideoSourceToken       string  `json:"videoSourceToken" binding:"required"`
		Text                   string  `json:"text" binding:"required"`
		X                      float64 `json:"x"`
		Y                      float64 `json:"y"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	cam, exists := h.cache.Get(req.EncryptedCameraDetails)
	if !exists {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "failed to retrieve camera"})
		return
	}

	h.log.Info("CreateOSD", zap.String("text", req.Text))

	token, err := mediaservice.CreateOSD(cam, req.VideoSourceToken, req.Text, req.X, req.Y)
	if err != nil {
		h.log.Error("failed to create OSD", zap.Error(err))
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "failed to create OSD"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"token": token})
}

// SetSynchronizationPoint handles POST /SetSynchronizationPoint
func (h *ONVIFClientHandler) SetSynchronizationPoint(c *gin.Context) {
	var req struct {
		EncryptedCameraDetails string `json:"encryptedCameraDetails" binding:"required"`
		ProfileToken           string `json:"profileToken" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	cam, exists := h.cache.Get(req.EncryptedCameraDetails)
	if !exists {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "failed to retrieve camera"})
		return
	}

	err := mediaservice.SetSynchronizationPoint(cam, req.ProfileToken)
	if err != nil {
		h.log.Error("failed to set sync point", zap.Error(err))
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "failed to set synchronization point"})
		return
	}

	c.Status(http.StatusOK)
}

// ModifyVideoEncoderResolution handles POST /ModifyVideoEncoderResolution
func (h *ONVIFClientHandler) ModifyVideoEncoderResolution(c *gin.Context) {
	var req struct {
		EncryptedCameraDetails string `json:"encryptedCameraDetails" binding:"required"`
		ConfigToken            string `json:"configToken" binding:"required"`
		NewWidth               int    `json:"newWidth" binding:"required"`
		NewHeight              int    `json:"newHeight" binding:"required"`
		NewFps                 int    `json:"newFps" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	cam, exists := h.cache.Get(req.EncryptedCameraDetails)
	if !exists {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "failed to retrieve camera"})
		return
	}

	h.log.Info("ModifyVideoEncoderResolution", zap.String("configToken", req.ConfigToken))

	err := mediaservice.ModifyVideoEncoderResolution(cam, req.ConfigToken, req.NewWidth, req.NewHeight, req.NewFps)
	if err != nil {
		h.log.Error("failed to modify resolution", zap.Error(err))
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "failed to modify video encoder resolution"})
		return
	}

	c.Status(http.StatusOK)
}

// ModifyVideoEncoderQuality handles POST /ModifyVideoEncoderQuality
func (h *ONVIFClientHandler) ModifyVideoEncoderQuality(c *gin.Context) {
	var req struct {
		EncryptedCameraDetails string `json:"encryptedCameraDetails" binding:"required"`
		ConfigToken            string `json:"configToken" binding:"required"`
		Bitrate                int    `json:"bitrate" binding:"required"`
		GovLength              int    `json:"govLength" binding:"required"`
		H264Profile            string `json:"h264Profile" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	cam, exists := h.cache.Get(req.EncryptedCameraDetails)
	if !exists {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "failed to retrieve camera"})
		return
	}

	h.log.Info("ModifyVideoEncoderQuality", zap.String("configToken", req.ConfigToken))

	err := mediaservice.ModifyVideoEncoderQuality(cam, req.ConfigToken, req.Bitrate, req.GovLength, req.H264Profile)
	if err != nil {
		h.log.Error("failed to modify quality", zap.Error(err))
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "failed to modify video encoder quality"})
		return
	}

	c.Status(http.StatusOK)
}

// --- PTZ Handlers ---

// GetPTZStatus handles GET /GetPTZStatus
func (h *ONVIFClientHandler) GetPTZStatus(c *gin.Context) {
	encryptedCameraDetails := c.Query("encryptedCameraDetails")
	profileToken := c.Query("profileToken")

	if encryptedCameraDetails == "" || profileToken == "" {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "encryptedCameraDetails and profileToken are required"})
		return
	}

	cam, exists := h.cache.Get(encryptedCameraDetails)
	if !exists {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "failed to retrieve camera"})
		return
	}

	h.log.Info("GetPTZStatus", zap.String("profileToken", profileToken))

	status, err := ptzservice.GetPTZStatus(cam, profileToken)
	if err != nil {
		h.log.Error("failed to get PTZ status", zap.Error(err))
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "failed to get PTZ status"})
		return
	}

	c.JSON(http.StatusOK, status)
}

// GetPTZConfigurations handles GET /GetPTZConfigurations
func (h *ONVIFClientHandler) GetPTZConfigurations(c *gin.Context) {
	encryptedCameraDetails := c.Query("encryptedCameraDetails")

	if encryptedCameraDetails == "" {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "encryptedCameraDetails is required"})
		return
	}

	cam, exists := h.cache.Get(encryptedCameraDetails)
	if !exists {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "failed to retrieve camera"})
		return
	}

	h.log.Info("GetPTZConfigurations", zap.String("encryptedCameraDetails", encryptedCameraDetails))

	configs, err := ptzservice.GetPTZConfigurations(cam)
	if err != nil {
		h.log.Error("failed to get PTZ configurations", zap.Error(err))
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "failed to get PTZ configurations"})
		return
	}

	c.JSON(http.StatusOK, configs)
}

// GetPresets handles GET /GetPresets
func (h *ONVIFClientHandler) GetPresets(c *gin.Context) {
	encryptedCameraDetails := c.Query("encryptedCameraDetails")
	profileToken := c.Query("profileToken")
	if encryptedCameraDetails == "" || profileToken == "" {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "encryptedCameraDetails and profileToken are required"})
		return
	}
	cam, exists := h.cache.Get(encryptedCameraDetails)
	if !exists {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "failed to retrieve camera"})
		return
	}
	h.log.Info("GetPresets", zap.String("details", encryptedCameraDetails), zap.String("profile", profileToken))

	presets, err := ptzservice.GetPresets(cam, profileToken)
	if err != nil {
		h.log.Error("failed to get presets", zap.Error(err))
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "failed to get presets"})
		return
	}
	c.JSON(http.StatusOK, presets)
}

func (h *ONVIFClientHandler) GetPresetTokenByName(c *gin.Context) {
	encryptedCameraDetails := c.Query("encryptedCameraDetails")
	profileToken := c.Query("profileToken")
	searchName := c.Query("searchName")
	if encryptedCameraDetails == "" || profileToken == "" || searchName == "" {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "details, profileToken, and searchName are required"})
		return
	}
	cam, exists := h.cache.Get(encryptedCameraDetails)
	if !exists {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "failed to retrieve camera"})
		return
	}

	token, err := ptzservice.GetPresetTokenByName(cam, profileToken, searchName)
	if err != nil {
		h.log.Error("failed to get preset token", zap.Error(err))
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "failed to get preset token by name"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"token": token})
}

func (h *ONVIFClientHandler) SetPreset(c *gin.Context) {
	var req struct {
		EncryptedCameraDetails string `json:"encryptedCameraDetails" binding:"required"`
		ProfileToken           string `json:"profileToken" binding:"required"`
		Name                   string `json:"name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	cam, exists := h.cache.Get(req.EncryptedCameraDetails)
	if !exists {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "failed to retrieve camera"})
		return
	}

	token, err := ptzservice.SetPreset(cam, req.ProfileToken, req.Name)
	if err != nil {
		h.log.Error("failed to set preset", zap.Error(err))
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "failed to set preset"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"token": token})
}

func (h *ONVIFClientHandler) RemovePreset(c *gin.Context) {
	var req struct {
		EncryptedCameraDetails string `json:"encryptedCameraDetails" binding:"required"`
		ProfileToken           string `json:"profileToken" binding:"required"`
		PresetToken            string `json:"presetToken" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	cam, exists := h.cache.Get(req.EncryptedCameraDetails)
	if !exists {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "failed to retrieve camera"})
		return
	}

	err := ptzservice.RemovePreset(cam, req.ProfileToken, req.PresetToken)
	if err != nil {
		h.log.Error("failed to remove preset", zap.Error(err))
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "failed to remove preset"})
		return
	}
	c.Status(http.StatusOK)
}

func (h *ONVIFClientHandler) AbsoluteMove(c *gin.Context) {
	var req struct {
		EncryptedCameraDetails string  `json:"encryptedCameraDetails" binding:"required"`
		Token                  string  `json:"token" binding:"required"`
		Pan                    float64 `json:"pan"`
		Tilt                   float64 `json:"tilt"`
		Zoom                   float64 `json:"zoom"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	cam, exists := h.cache.Get(req.EncryptedCameraDetails)
	if !exists {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "failed to retrieve camera"})
		return
	}

	err := ptzservice.AbsoluteMove(cam, req.Token, req.Pan, req.Tilt, req.Zoom)
	if err != nil {
		h.log.Error("absolute move failed", zap.Error(err))
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "failed to perform absolute move"})
		return
	}
	c.Status(http.StatusOK)
}

func (h *ONVIFClientHandler) RelativeMove(c *gin.Context) {
	var req struct {
		EncryptedCameraDetails string  `json:"encryptedCameraDetails" binding:"required"`
		Token                  string  `json:"token" binding:"required"`
		XDist                  float64 `json:"xDist"`
		YDist                  float64 `json:"yDist"`
		ZoomDist               float64 `json:"zoomDist"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	cam, exists := h.cache.Get(req.EncryptedCameraDetails)
	if !exists {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "failed to retrieve camera"})
		return
	}

	err := ptzservice.RelativeMove(cam, req.Token, req.XDist, req.YDist, req.ZoomDist)
	if err != nil {
		h.log.Error("relative move failed", zap.Error(err))
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "failed to perform relative move"})
		return
	}
	c.Status(http.StatusOK)
}

func (h *ONVIFClientHandler) ContinuousMove(c *gin.Context) {
	var req struct {
		EncryptedCameraDetails string  `json:"encryptedCameraDetails" binding:"required"`
		Token                  string  `json:"token" binding:"required"`
		PanSpeed               float64 `json:"panSpeed"`
		TiltSpeed              float64 `json:"tiltSpeed"`
		ZoomSpeed              float64 `json:"zoomSpeed"`
		Timeout                int64   `json:"timeout"` // Milliseconds
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	cam, exists := h.cache.Get(req.EncryptedCameraDetails)
	if !exists {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "failed to retrieve camera"})
		return
	}

	err := ptzservice.ContinuousMove(cam, req.Token, req.PanSpeed, req.TiltSpeed, req.ZoomSpeed, time.Duration(req.Timeout)*time.Millisecond)
	if err != nil {
		h.log.Error("continuous move failed", zap.Error(err))
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "failed to perform continuous move"})
		return
	}
	c.Status(http.StatusOK)
}

func (h *ONVIFClientHandler) StopPTZ(c *gin.Context) {
	var req struct {
		EncryptedCameraDetails string `json:"encryptedCameraDetails" binding:"required"`
		Token                  string `json:"token" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	cam, exists := h.cache.Get(req.EncryptedCameraDetails)
	if !exists {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "failed to retrieve camera"})
		return
	}

	err := ptzservice.StopPTZ(cam, req.Token)
	if err != nil {
		h.log.Error("stop PTZ failed", zap.Error(err))
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "failed to stop PTZ"})
		return
	}
	c.Status(http.StatusOK)
}

func (h *ONVIFClientHandler) GetPTZNodes(c *gin.Context) {
	encryptedCameraDetails := c.Query("encryptedCameraDetails")
	if encryptedCameraDetails == "" {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "encryptedCameraDetails is required"})
		return
	}
	cam, exists := h.cache.Get(encryptedCameraDetails)
	if !exists {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "failed to retrieve camera"})
		return
	}

	nodes, err := ptzservice.GetPTZNodes(cam)
	if err != nil {
		h.log.Error("failed to get nodes", zap.Error(err))
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "failed to get PTZ nodes"})
		return
	}
	c.JSON(http.StatusOK, nodes)
}

func (h *ONVIFClientHandler) GetPresetTours(c *gin.Context) {
	encryptedCameraDetails := c.Query("encryptedCameraDetails")
	profileToken := c.Query("profileToken")
	if encryptedCameraDetails == "" || profileToken == "" {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "details and profileToken are required"})
		return
	}
	cam, exists := h.cache.Get(encryptedCameraDetails)
	if !exists {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "failed to retrieve camera"})
		return
	}

	tours, err := ptzservice.GetPresetTours(cam, profileToken)
	if err != nil {
		h.log.Error("failed to get tours", zap.Error(err))
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "failed to get preset tours"})
		return
	}
	c.JSON(http.StatusOK, tours)
}

func (h *ONVIFClientHandler) OperatePresetTour(c *gin.Context) {
	var req struct {
		EncryptedCameraDetails string `json:"encryptedCameraDetails" binding:"required"`
		ProfileToken           string `json:"profileToken" binding:"required"`
		TourToken              string `json:"tourToken" binding:"required"`
		Operation              string `json:"operation" binding:"required"` // Start, Stop, Pause
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	cam, exists := h.cache.Get(req.EncryptedCameraDetails)
	if !exists {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "failed to retrieve camera"})
		return
	}

	err := ptzservice.OperatePresetTour(cam, req.ProfileToken, req.TourToken, req.Operation)
	if err != nil {
		h.log.Error("tour operation failed", zap.Error(err))
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "failed to operate preset tour"})
		return
	}
	c.Status(http.StatusOK)
}

func (h *ONVIFClientHandler) GotoHomePosition(c *gin.Context) {
	var req struct {
		EncryptedCameraDetails string  `json:"encryptedCameraDetails" binding:"required"`
		ProfileToken           string  `json:"profileToken" binding:"required"`
		Speed                  float64 `json:"speed"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	cam, exists := h.cache.Get(req.EncryptedCameraDetails)
	if !exists {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "failed to retrieve camera"})
		return
	}

	err := ptzservice.GotoHomePosition(cam, req.ProfileToken, req.Speed)
	if err != nil {
		h.log.Error("goto home failed", zap.Error(err))
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "failed to go to home position"})
		return
	}
	c.Status(http.StatusOK)
}

func (h *ONVIFClientHandler) SetHomePosition(c *gin.Context) {
	var req struct {
		EncryptedCameraDetails string `json:"encryptedCameraDetails" binding:"required"`
		ProfileToken           string `json:"profileToken" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	cam, exists := h.cache.Get(req.EncryptedCameraDetails)
	if !exists {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "failed to retrieve camera"})
		return
	}

	err := ptzservice.SetHomePosition(cam, req.ProfileToken)
	if err != nil {
		h.log.Error("set home failed", zap.Error(err))
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "failed to set home position"})
		return
	}
	c.Status(http.StatusOK)
}

func (h *ONVIFClientHandler) GotoPreset(c *gin.Context) {
	var req struct {
		EncryptedCameraDetails string `json:"encryptedCameraDetails" binding:"required"`
		ProfileToken           string `json:"profileToken" binding:"required"`
		PresetToken            string `json:"presetToken" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	cam, exists := h.cache.Get(req.EncryptedCameraDetails)
	if !exists {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "failed to retrieve camera"})
		return
	}

	err := ptzservice.GotoPreset(cam, req.ProfileToken, req.PresetToken)
	if err != nil {
		h.log.Error("goto preset failed", zap.Error(err))
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "failed to go to preset"})
		return
	}
	c.Status(http.StatusOK)
}
