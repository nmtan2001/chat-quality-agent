package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/nmtan2001/chat-quality-agent/config"
)

const configKey = "config"

// ConfigInjector creates middleware that injects config into request context
func ConfigInjector(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(configKey, cfg)
		c.Next()
	}
}

// GetConfig retrieves config from Gin context
func GetConfig(c *gin.Context) *config.Config {
	if cfg, exists := c.Get(configKey); exists {
		return cfg.(*config.Config)
	}
	return nil
}
