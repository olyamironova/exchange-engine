package domain

import "time"

type OrderbookSnapshot struct {
	Bids      []Order
	Asks      []Order
	Timestamp time.Time
	Symbol    string
}
