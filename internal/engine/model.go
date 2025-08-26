package engine

import "time"

type Side string
type OrderType string

const (
	Buy  Side = "BUY"
	Sell Side = "SELL"

	Limit  OrderType = "LIMIT"
	Market OrderType = "MARKET"
)

type Order struct {
	ID        string    `json:"id"`
	ClientID  string    `json:"client_id"`
	Symbol    string    `json:"symbol"`
	Side      Side      `json:"side"`
	Type      OrderType `json:"type"`
	Price     float64   `json:"price"`
	Quantity  float64   `json:"quantity"`
	Remaining float64   `json:"remaining"`
	CreatedAt time.Time `json:"created_at"`
}

type Trade struct {
	ID        string    `json:"id"`
	BuyOrder  string    `json:"buy_order"`
	SellOrder string    `json:"sell_order"`
	Price     float64   `json:"price"`
	Quantity  float64   `json:"quantity"`
	Timestamp time.Time `json:"timestamp"`
}

type OrderbookSnapshot struct {
	Bids []Order `json:"bids"`
	Asks []Order `json:"asks"`
}
