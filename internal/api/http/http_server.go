package http

import (
	"fmt"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/olyamironova/exchange-engine/internal/api/dto"
	"github.com/olyamironova/exchange-engine/internal/core"
	"github.com/olyamironova/exchange-engine/internal/domain"
	"github.com/olyamironova/exchange-engine/internal/middleware"
)

type HTTPServer struct {
	Eng         *core.Engine
	submittedID sync.Map // for deduplication by OrderID
}

func NewHTTPServer(eng *core.Engine) *HTTPServer {
	return &HTTPServer{Eng: eng}
}

func (s *HTTPServer) Run(addr string) error {
	r := gin.Default()

	// Middleware rate-limiting
	rl := middleware.NewRateLimiter(time.Millisecond * 100)
	r.Use(rl.Middleware())

	r.POST("/orders", s.submitOrder)
	r.POST("/orders/modify", s.modifyOrder)
	r.POST("/orders/cancel", s.cancelOrder)
	r.GET("/orders/:id", s.getOrder)
	r.GET("/orders/:id/trades", s.getTrades)
	r.GET("/orderbook", s.getOrderbook)
	r.POST("/orderbook/snapshot", s.snapshotOrderbook)
	r.POST("/orderbook/restore", s.restoreOrderbook)

	return r.Run(addr)
}

func (s *HTTPServer) submitOrder(c *gin.Context) {
	var req dto.SubmitOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := ValidateOrder(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// deduplication
	if req.OrderID != "" {
		if _, exists := s.submittedID.LoadOrStore(req.OrderID, struct{}{}); exists {
			c.JSON(http.StatusOK, gin.H{"message": "duplicate order", "order_id": req.OrderID})
			return
		}
	}

	orderID := req.OrderID
	if orderID == "" {
		orderID = uuid.NewString()
	}

	o := &domain.Order{
		ID:       req.OrderID,
		ClientID: req.ClientID,
		Symbol:   req.Symbol,
		Side:     domain.Side(req.Side),
		Type:     domain.OrderType(req.Type),
		Price:    req.Price,
		Quantity: req.Quantity,
	}

	trades, err := s.Eng.SubmitOrder(c, o)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, dto.SubmitOrderResponse{
		OrderID:   o.ID,
		Trades:    convertTrades(trades),
		Remaining: o.Remaining,
	})
}

func (s *HTTPServer) modifyOrder(c *gin.Context) {
	var req dto.ModifyOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	ok, err := s.Eng.ModifyOrder(c, req.OrderID, req.ClientID, req.NewPrice, req.NewQty)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.ModifyOrderResponse{
		OrderID:  req.OrderID,
		Modified: ok,
	})
}

func (s *HTTPServer) cancelOrder(c *gin.Context) {
	var req dto.CancelOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	ok, err := s.Eng.CancelOrder(c, req.OrderID, req.ClientID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.CancelOrderResponse{
		OrderID:   req.OrderID,
		Cancelled: ok,
	})
}

func (s *HTTPServer) getOrder(c *gin.Context) {
	id := c.Param("id")
	o, err := s.Eng.GetOrder(c, id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.GetOrderResponse{Order: convertOrder(o)})
}

func (s *HTTPServer) getTrades(c *gin.Context) {
	id := c.Param("id")
	trades, _ := s.Eng.GetTradesForOrder(c.Request.Context(), id)
	c.JSON(http.StatusOK, dto.GetTradesResponse{Trades: convertTrades(trades)})
}

func (s *HTTPServer) getOrderbook(c *gin.Context) {
	symbol := c.Query("symbol")
	ob, err := s.Eng.GetOrderbook(c.Request.Context(), symbol)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	copySnapshot := ob.DeepCopy()
	c.JSON(http.StatusOK, dto.GetOrderbookResponse{
		Bids:      convertOrders(copySnapshot.Bids),
		Asks:      convertOrders(copySnapshot.Asks),
		Timestamp: copySnapshot.Timestamp,
	})
}

func (s *HTTPServer) snapshotOrderbook(c *gin.Context) {
	var req dto.SnapshotRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	id, err := s.Eng.SnapshotOrderbook(c.Request.Context(), req.Symbol)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.SnapshotResponse{SnapshotID: id})
}

func (s *HTTPServer) restoreOrderbook(c *gin.Context) {
	var req dto.RestoreRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	ok, err := s.Eng.RestoreOrderbook(c.Request.Context(), req.SnapshotID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.RestoreResponse{Ok: ok})
}

func convertOrder(o *domain.Order) dto.Order {
	return dto.Order{
		ID:        o.ID,
		ClientID:  o.ClientID,
		Symbol:    o.Symbol,
		Side:      dto.Side(o.Side),
		Type:      dto.OrderType(o.Type),
		Price:     o.Price,
		Quantity:  o.Quantity,
		Remaining: o.Remaining,
		Status:    string(o.Status),
		CreatedAt: o.CreatedAt,
	}
}

func convertOrders(orders []domain.Order) []dto.Order {
	res := make([]dto.Order, len(orders))
	for i, o := range orders {
		res[i] = convertOrder(&o)
	}
	return res
}

func convertTrades(trades []*domain.Trade) []dto.Trade {
	res := make([]dto.Trade, len(trades))
	for i, t := range trades {
		res[i] = dto.Trade{
			ID:        t.ID,
			BuyOrder:  t.BuyOrder,
			SellOrder: t.SellOrder,
			Price:     t.Price,
			Quantity:  t.Quantity,
			Timestamp: t.Timestamp,
		}
	}
	return res
}

func TimeToProto(t time.Time) *timestamppb.Timestamp {
	return timestamppb.New(t)
}

func ValidateOrder(req *dto.SubmitOrderRequest) error {
	switch req.Side {
	case dto.Buy, dto.Sell:
	default:
		return fmt.Errorf("invalid side: %s", req.Side)
	}
	switch req.Type {
	case dto.Limit, dto.Market:
	default:
		return fmt.Errorf("invalid order type: %s", req.Type)
	}
	if req.Quantity <= 0 {
		return fmt.Errorf("quantity must be > 0")
	}
	if req.Type == dto.Limit && req.Price <= 0 {
		return fmt.Errorf("price must be > 0 for LIMIT orders")
	}
	return nil
}
