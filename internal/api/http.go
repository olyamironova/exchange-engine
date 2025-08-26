package api

import (
	"github.com/gin-gonic/gin"
	"github.com/olyamironova/exchange-engine/proto"
	"net/http"
)

type HTTPServer struct {
	exchangeClient proto.ExchangeClient
}

func NewHTTPServer(client proto.ExchangeClient) *HTTPServer {
	return &HTTPServer{exchangeClient: client}
}

func (s *HTTPServer) Run(addr string) error {
	r := gin.Default()

	r.POST("/orders", s.submitOrderHandler)
	r.GET("/orderbook", s.getOrderbookHandler)
	r.POST("/cancel", s.cancelOrderHandler)
	r.GET("/trades", s.getTradesHandler)

	return r.Run(addr)
}

func (s *HTTPServer) submitOrderHandler(c *gin.Context) {
	var req proto.SubmitOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	resp, err := s.exchangeClient.SubmitOrder(c, &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (s *HTTPServer) getOrderbookHandler(c *gin.Context) {
	symbol := c.Query("symbol")
	if symbol == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "symbol required"})
		return
	}

	resp, err := s.exchangeClient.GetOrderbook(c, &proto.GetOrderbookRequest{Symbol: symbol})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (s *HTTPServer) cancelOrderHandler(c *gin.Context) {
	var req proto.CancelOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	resp, err := s.exchangeClient.CancelOrder(c, &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (s *HTTPServer) getTradesHandler(c *gin.Context) {
	symbol := c.Query("symbol")
	resp, err := s.exchangeClient.GetTrades(c, &proto.GetTradesRequest{Symbol: symbol})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, resp)
}
