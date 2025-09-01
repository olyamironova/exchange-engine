package domain

import (
	"github.com/shopspring/decimal"
	"time"
)

type Side string
type OrderType string
type OrderStatus string

const (
	Buy             Side        = "BUY"
	Sell            Side        = "SELL"
	Limit           OrderType   = "LIMIT"
	Market          OrderType   = "MARKET"
	Open            OrderStatus = "OPEN"
	Filled          OrderStatus = "FILLED"
	Cancelled       OrderStatus = "CANCELLED"
	PartiallyFilled OrderStatus = "PARTIALLY FILLED"
)

type Order struct {
	ID             string
	ClientID       string
	ClientOrderID  string
	Symbol         string
	Side           Side
	Type           OrderType
	Price          decimal.Decimal
	Quantity       decimal.Decimal
	FilledQuantity decimal.Decimal
	Remaining      decimal.Decimal
	Status         OrderStatus
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (o *Order) PartiallyFilled() bool {
	return o.FilledQuantity.GreaterThan(decimal.Zero) &&
		o.FilledQuantity.LessThan(o.Quantity)
}
