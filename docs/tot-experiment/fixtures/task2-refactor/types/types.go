package types

type User struct {
	ID       string
	Email    string
	Name     string
	Role     string
	Verified bool
}

type Order struct {
	ID        string
	UserID    string
	Items     []OrderItem
	Status    string
	Total     float64
	CreatedAt string
}

type OrderItem struct {
	ProductID string
	Name      string
	Price     float64
	Quantity  int
}

type Notification struct {
	To      string
	Subject string
	Body    string
}
