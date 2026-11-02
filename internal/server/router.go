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

		// Ensure backend secret is configured.
		if expectedSecret == "" {
			log.Println("[Router Auth] ERROR: BACKEND_SECRET is not configured")

			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"error": "Internal server error",
			})
			return
		}

		// Read the API key sent by Vaani.
		clientSecret := strings.TrimSpace(c.GetHeader("X-API-KEY"))

		// Log request details without exposing secrets.
		log.Printf(
			"[Router Auth] %s %s - X-API-KEY present: %t",
			c.Request.Method,
			c.Request.URL.Path,
			clientSecret != "",
		)

		// Check whether the API key is present.
		if clientSecret == "" {
			log.Println("[Router Auth] FAILED: X-API-KEY header is missing")

			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "Unauthorized",
			})
			return
		}

		// Compare provided key with backend secret.
		if subtle.ConstantTimeCompare(
			[]byte(clientSecret),
			[]byte(expectedSecret),
		) != 1 {

			log.Println("[Router Auth] FAILED: API key does not match")

			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "Unauthorized",
			})
			return
		}

		log.Println("[Router Auth] SUCCESS: Request authenticated")

		c.Next()
	}
}
