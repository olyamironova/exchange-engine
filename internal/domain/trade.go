package domain

import "time"

type Trade struct {
	ID        string
	Symbol    string
	BuyOrder  string
	SellOrder string
	Price     float64
	Quantity  float64
	Timestamp time.Time
}
