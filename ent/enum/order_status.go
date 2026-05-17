package enum

type OrderStatus string

const (
	Created   OrderStatus = "CREATED"
	Confirmed OrderStatus = "CONFIRMED"
	Shipped   OrderStatus = "SHIPPED"
	Accepted  OrderStatus = "ACCEPTED"
	Rejected  OrderStatus = "REJECTED"
)

func (OrderStatus) Values() []string {
	return []string{
		string(Created),
		string(Confirmed),
		string(Shipped),
		string(Accepted),
		string(Rejected),
	}
}

func (s OrderStatus) Value() string {
	return string(s)
}

func (s OrderStatus) IsValid() bool {
	switch s {
	case Created, Confirmed, Shipped, Accepted, Rejected:
		return true
	}
	return false
}
