package domain

import "time"

type Side string
type OrderType string
type OrderStatus string

const (
	Buy       Side        = "BUY"
	Sell      Side        = "SELL"
	Limit     OrderType   = "LIMIT"
	Market    OrderType   = "MARKET"
	Open      OrderStatus = "OPEN"
	Filled    OrderStatus = "FILLED"
	Cancelled OrderStatus = "CANCELLED"
)

type Order struct {
	ID            string
	ClientID      string
	ClientOrderID string
	Symbol        string
	Side          Side
	Type          OrderType
	Price         float64
	Quantity      float64
	Remaining     float64
	Status        OrderStatus
	CreatedAt     time.Time
}
