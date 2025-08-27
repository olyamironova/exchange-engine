package main

import (
	"context"
	"log"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/olyamironova/exchange-engine/internal/adapter/pg"
	"github.com/olyamironova/exchange-engine/internal/core"
	"github.com/olyamironova/exchange-engine/internal/domain"
)

func main() {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://postgres:postgres@localhost:5432/exchange?sslmode=disable"
	}

	ctx := context.Background()
	repo, err := pg.NewPgRepo(ctx, dsn)
	if err != nil {
		log.Fatal("failed to connect to db:", err)
	}
	defer repo.Close(ctx)

	engine := core.NewEngine(repo, nil)

	r := gin.Default()

	r.POST("/order", func(c *gin.Context) {
		var req struct {
			ClientID string  `json:"client_id"`
			Symbol   string  `json:"symbol"`
			Side     string  `json:"side"`
			Type     string  `json:"type"`
			Price    float64 `json:"price"`
			Quantity float64 `json:"quantity"`
		}
		if err := c.BindJSON(&req); err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}

		order := &domain.Order{
			ClientID: req.ClientID,
			Symbol:   req.Symbol,
			Side:     domain.Side(req.Side),
			Type:     domain.OrderType(req.Type),
			Price:    req.Price,
			Quantity: req.Quantity,
		}

		trades, err := engine.SubmitOrder(ctx, order)
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}

		c.JSON(200, gin.H{
			"order_id":  order.ID,
			"remaining": order.Remaining,
			"trades":    trades,
		})
	})

	r.GET("/orderbook", func(c *gin.Context) {
		symbol := c.Query("symbol")
		ob, err := engine.GetOrderbook(ctx, symbol)
		if err != nil {
			c.JSON(404, gin.H{"error": err.Error()})
			return
		}
		c.JSON(200, ob)
	})

	log.Println("server listening on :8080")
	r.Run(":8080")
}
