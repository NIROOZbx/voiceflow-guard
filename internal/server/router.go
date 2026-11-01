package server

import "github.com/gin-gonic/gin"

func NewRouter(bookingHandler *BookingHandler) *gin.Engine {
	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery())
	router.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	if bookingHandler != nil {
		router.POST("/bookings", bookingHandler.CreateBooking)
		router.GET("/bookings", bookingHandler.GetBooking)
	}

	return router
}
