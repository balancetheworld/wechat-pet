package httpapi

import (
	"crypto/rand"
	"encoding/hex"

	"github.com/gin-gonic/gin"
)

const requestIDKey = "request_id"

func requestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader("X-Request-ID")
		if id == "" {
			var bytes [16]byte
			if _, err := rand.Read(bytes[:]); err == nil {
				id = hex.EncodeToString(bytes[:])
			} else {
				id = "unknown"
			}
		}
		c.Set(requestIDKey, id)
		c.Header("X-Request-ID", id)
		c.Next()
	}
}
