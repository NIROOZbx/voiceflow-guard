package server

import (
	"crypto/subtle"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

func NewRouter(bookingHandler *BookingHandler) *gin.Engine {
	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery())

	router.GET("/health", func(c *gin.Context) {
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

		// Ensure the backend secret is configured.
		if expectedSecret == "" {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"error": "Internal server error",
			})
			return
		}

		// Read the API key sent by Vaani.
		clientSecret := c.GetHeader("X-API-KEY")

		// Validate the key.
		if clientSecret == "" ||
			subtle.ConstantTimeCompare(
				[]byte(clientSecret),
				[]byte(expectedSecret),
			) != 1 {

			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "Unauthorized",
			})
			return
		}

		// Authentication successful.
		c.Next()
	}
}
