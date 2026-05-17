package data

import (
	"github.com/shopspring/decimal"
)

type OrderDto struct {
	ID             int64
	TenantID       int64
	CounterpartyID int64
	CreatedBy      int64
}

type OrderItemDto struct {
	OrderID   int64
	ProductID int64
	Quantity  decimal.Decimal
	UnitPrice decimal.Decimal
}
