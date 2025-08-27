package dto

import "time"

type OrderType string

const (
	Limit  OrderType = "LIMIT"
	Market OrderType = "MARKET"
)

type Side string

const (
	Buy  Side = "BUY"
	Sell Side = "SELL"
)

type SubmitOrderRequest struct {
	OrderID  string    `json:"order_id,omitempty"` // for deduplicate
	ClientID string    `json:"client_id" binding:"required"`
	Symbol   string    `json:"symbol" binding:"required"`
	Side     Side      `json:"side" binding:"required"`
	Type     OrderType `json:"type" binding:"required"`
	Price    float64   `json:"price,omitempty"` // for limited order
	Quantity float64   `json:"quantity" binding:"required"`
}

type SubmitOrderResponse struct {
	OrderID   string  `json:"order_id"`
	Trades    []Trade `json:"trades"`
	Remaining float64 `json:"remaining"`
	Message   string  `json:"message,omitempty"`
}

type ModifyOrderRequest struct {
	OrderID  string  `json:"order_id" binding:"required"`
	ClientID string  `json:"client_id" binding:"required"`
	NewPrice float64 `json:"new_price,omitempty"`
	NewQty   float64 `json:"new_qty,omitempty"`
}

type ModifyOrderResponse struct {
	OrderID  string `json:"order_id"`
	Modified bool   `json:"modified"`
	Message  string `json:"message,omitempty"`
}

type CancelOrderRequest struct {
	OrderID  string `json:"order_id" binding:"required"`
	ClientID string `json:"client_id" binding:"required"`
}

type CancelOrderResponse struct {
	OrderID   string `json:"order_id"`
	Cancelled bool   `json:"cancelled"`
	Message   string `json:"message,omitempty"`
}

type GetOrderRequest struct {
	OrderID string `json:"order_id" binding:"required"`
}

type GetOrderResponse struct {
	Order Order `json:"order"`
}

type GetTradesRequest struct {
	OrderID string `json:"order_id" binding:"required"`
}

type GetTradesResponse struct {
	Trades []Trade `json:"trades"`
}

type GetOrderbookRequest struct {
	Symbol string `form:"symbol" binding:"required"`
}

type GetOrderbookResponse struct {
	Bids      []Order   `json:"bids"`
	Asks      []Order   `json:"asks"`
	Timestamp time.Time `json:"timestamp"`
}

type SnapshotRequest struct {
	Symbol string `json:"symbol" binding:"required"`
}

type SnapshotResponse struct {
	SnapshotID string `json:"snapshot_id"`
	Message    string `json:"message,omitempty"`
}

type RestoreRequest struct {
	SnapshotID string `json:"snapshot_id" binding:"required"`
}

type RestoreResponse struct {
	Ok      bool   `json:"ok"`
	Message string `json:"message,omitempty"`
}

// Core DTO
type Order struct {
	ID        string    `json:"id"`
	ClientID  string    `json:"client_id"`
	Symbol    string    `json:"symbol"`
	Side      Side      `json:"side"`
	Type      OrderType `json:"type"`
	Price     float64   `json:"price"`
	Quantity  float64   `json:"quantity"`
	Remaining float64   `json:"remaining"`
	Status    string    `json:"status"`
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
