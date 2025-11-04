package main

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	pb "github.com/GoogleCloudPlatform/microservices-demo/src/checkoutservice/genproto"
)

type OrderStore struct {
	db *sql.DB
}

type Order struct {
	OrderID                   string
	UserID                    string
	Email                     string
	StreetAddress             string
	City                      string
	State                     string
	Country                   string
	ZipCode                   string
	CreditCardNumber          string
	CreditCardCVV             string
	CreditCardExpirationMonth int32
	CreditCardExpirationYear  int32
	OrderTotal                float64
	CurrencyCode              string
	ShippingTrackingID        string
	CreatedAt                 time.Time
}

type OrderItem struct {
	ID        int
	OrderID   string
	ProductID string
	Quantity  int32
}

// creates a new order store
func NewOrderStore(db *sql.DB) *OrderStore {
	return &OrderStore{db: db}
}

// persists an order to the database
func (os *OrderStore) SaveOrder(ctx context.Context, orderID, userID, email string,
	address *pb.Address, creditCard *pb.CreditCardInfo, total *pb.Money,
	items []*pb.CartItem, trackingID string) error {

	tx, err := os.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	maskedCard := maskCreditCard(creditCard.CreditCardNumber)

	insertOrderSQL := `
        INSERT INTO orders (
            order_id, user_id, email, street_address, city, state, country, zip_code,
            credit_card_number, credit_card_cvv, credit_card_expiration_month,
            credit_card_expiration_year, order_total, currency_code, shipping_tracking_id, created_at
        ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
    `

	orderTotal := float64(total.Units) + float64(total.Nanos)/1e9

	_, err = tx.ExecContext(ctx, insertOrderSQL,
		orderID,
		userID,
		email,
		address.StreetAddress,
		address.City,
		address.State,
		address.Country,
		address.ZipCode,
		maskedCard,
		creditCard.CreditCardCvv,
		creditCard.CreditCardExpirationMonth,
		creditCard.CreditCardExpirationYear,
		orderTotal,
		total.CurrencyCode,
		trackingID,
		time.Now(),
	)

	if err != nil {
		return fmt.Errorf("failed to insert order: %w", err)
	}

	insertItemSQL := `
        INSERT INTO order_items (order_id, product_id, quantity)
        VALUES ($1, $2, $3)
    `

	for _, item := range items {
		_, err = tx.ExecContext(ctx, insertItemSQL,
			orderID,
			item.ProductId,
			item.Quantity,
		)
		if err != nil {
			return fmt.Errorf("failed to insert order item: %w", err)
		}
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	log.Infof("Order %s persisted to database successfully", orderID)
	return nil
}

// retrieves an order from the database
func (os *OrderStore) GetOrder(ctx context.Context, orderID string) (*Order, error) {
	order := &Order{}

	query := `
        SELECT order_id, user_id, email, street_address, city, state, country, zip_code,
               credit_card_number, credit_card_cvv, credit_card_expiration_month,
               credit_card_expiration_year, order_total, currency_code, shipping_tracking_id, created_at
        FROM orders WHERE order_id = $1
    `

	err := os.db.QueryRowContext(ctx, query, orderID).Scan(
		&order.OrderID,
		&order.UserID,
		&order.Email,
		&order.StreetAddress,
		&order.City,
		&order.State,
		&order.Country,
		&order.ZipCode,
		&order.CreditCardNumber,
		&order.CreditCardCVV,
		&order.CreditCardExpirationMonth,
		&order.CreditCardExpirationYear,
		&order.OrderTotal,
		&order.CurrencyCode,
		&order.ShippingTrackingID,
		&order.CreatedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("order not found: %s", orderID)
		}
		return nil, fmt.Errorf("failed to query order: %w", err)
	}

	return order, nil
}

// retrieves all orders for a user
func (os *OrderStore) GetUserOrders(ctx context.Context, userID string) ([]Order, error) {
	query := `
        SELECT order_id, user_id, email, street_address, city, state, country, zip_code,
               credit_card_number, credit_card_cvv, credit_card_expiration_month,
               credit_card_expiration_year, order_total, currency_code, shipping_tracking_id, created_at
        FROM orders WHERE user_id = $1 ORDER BY created_at DESC
    `

	rows, err := os.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to query user orders: %w", err)
	}
	defer rows.Close()

	var orders []Order
	for rows.Next() {
		order := Order{}
		err := rows.Scan(
			&order.OrderID,
			&order.UserID,
			&order.Email,
			&order.StreetAddress,
			&order.City,
			&order.State,
			&order.Country,
			&order.ZipCode,
			&order.CreditCardNumber,
			&order.CreditCardCVV,
			&order.CreditCardExpirationMonth,
			&order.CreditCardExpirationYear,
			&order.OrderTotal,
			&order.CurrencyCode,
			&order.ShippingTrackingID,
			&order.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan order: %w", err)
		}
		orders = append(orders, order)
	}

	return orders, rows.Err()
}

// masks all but last 4 digits
func maskCreditCard(cardNumber string) string {
	if len(cardNumber) < 4 {
		return "****"
	}
	return "****-****-****-" + cardNumber[len(cardNumber)-4:]
}

// InitDB initializes database schema
func InitDB(db *sql.DB) error {
	createOrdersTable := `
        CREATE TABLE IF NOT EXISTS orders (
            order_id VARCHAR(50) PRIMARY KEY,
            user_id VARCHAR(50) NOT NULL,
            email VARCHAR(255),
            street_address VARCHAR(500),
            city VARCHAR(100),
            state VARCHAR(100),
            country VARCHAR(100),
            zip_code VARCHAR(20),
            credit_card_number VARCHAR(25),
            credit_card_cvv VARCHAR(4),
            credit_card_expiration_month INT,
            credit_card_expiration_year INT,
            order_total DECIMAL(10, 2),
            currency_code VARCHAR(3),
            shipping_tracking_id VARCHAR(100),
            created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
        );
        CREATE INDEX IF NOT EXISTS idx_orders_user_id ON orders(user_id);
        CREATE INDEX IF NOT EXISTS idx_orders_created_at ON orders(created_at);
    `

	createOrderItemsTable := `
        CREATE TABLE IF NOT EXISTS order_items (
            id SERIAL PRIMARY KEY,
            order_id VARCHAR(50) NOT NULL REFERENCES orders(order_id) ON DELETE CASCADE,
            product_id VARCHAR(50) NOT NULL,
            quantity INT NOT NULL
        );
        CREATE INDEX IF NOT EXISTS idx_order_items_order_id ON order_items(order_id);
    `

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := db.ExecContext(ctx, createOrdersTable)
	if err != nil {
		return fmt.Errorf("failed to create orders table: %w", err)
	}

	_, err = db.ExecContext(ctx, createOrderItemsTable)
	if err != nil {
		return fmt.Errorf("failed to create order_items table: %w", err)
	}

	log.Info("Database schema initialized successfully")
	return nil
}
