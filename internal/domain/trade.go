package domain

import (
	"github.com/shopspring/decimal"
	"time"
)

type Trade struct {
	ID        string
	Symbol    string
	BuyOrder  string
	SellOrder string
	Price     decimal.Decimal
	Quantity  decimal.Decimal
	Timestamp time.Time
}
