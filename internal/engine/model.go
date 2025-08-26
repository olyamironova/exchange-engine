package engine

import "time"

type Side string
type OrderType string
type OrderStatus string

const (
	Buy  Side = "BUY"
	Sell Side = "SELL"

	Limit  OrderType = "LIMIT"
	Market OrderType = "MARKET"

	Open     OrderStatus = "OPEN"
	Filled   OrderStatus = "FILLED"
	Canceled OrderStatus = "CANCELED"
)

type Order struct {
	ID        string
	ClientID  string
	Symbol    string
	Side      Side
	Type      OrderType
	Price     float64
	Quantity  float64
	Remaining float64
	Status    OrderStatus
	CreatedAt time.Time
}

type Trade struct {
	ID        string
	BuyOrder  string
	SellOrder string
	Price     float64
	Quantity  float64
	Timestamp time.Time
}

type OrderbookSnapshot struct {
	Bids []Order
	Asks []Order
}
