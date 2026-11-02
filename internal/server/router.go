package server

import (
	"crypto/subtle"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

func NewRouter(bookingHandler *BookingHandler) *gin.Engine {
	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery())

	router.GET("/health", func(c *gin.Context) {
		log.Println("[Router] Handling /health check")
		c.JSON(200, gin.H{"status": "ok"})
	})

	if bookingHandler != nil {
		bookingRoutes := router.Group("/bookings")
		bookingRoutes.Use(BackendSecretAuth())

		bookingRoutes.POST("", bookingHandler.CreateBooking)
		bookingRoutes.GET("", bookingHandler.GetBooking)
	}

	return router
}

func BackendSecretAuth() gin.HandlerFunc {
	expectedSecret := strings.TrimSpace(os.Getenv("BACKEND_SECRET"))

	return func(c *gin.Context) {
		clientSecret := c.GetHeader("X-API-KEY")
		backendSecret := c.GetHeader("X-Backend-Secret")

		log.Printf("[Router Auth] %s %s - Headers: X-API-KEY=%q, X-Backend-Secret=%q, Idempotency-Key=%q",
			c.Request.Method, c.Request.URL.Path, clientSecret, backendSecret, c.GetHeader("Idempotency-Key"))

		// Ensure the backend secret is configured.
		if expectedSecret == "" {
			log.Println("[Router Auth] ERROR: BACKEND_SECRET environment variable is not configured")
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"error": "Internal server error",
			})
			return
		}

		// Validate the key.
		if clientSecret == "" {
			log.Printf("[Router Auth] FAILED: X-API-KEY header is missing (received X-Backend-Secret=%q)", backendSecret)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "Unauthorized",
			})
			return
		}

		if subtle.ConstantTimeCompare([]byte(clientSecret), []byte(expectedSecret)) != 1 {
			log.Printf("[Router Auth] FAILED: X-API-KEY does not match expected secret")
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "Unauthorized",
			})
			return
		}

		// Authentication successful.
		log.Println("[Router Auth] SUCCESS: Request authenticated")
		c.Next()
	}
}
