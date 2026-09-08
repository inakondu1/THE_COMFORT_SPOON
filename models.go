package main

type Product struct {
	ID          int
	Name        string
	Description string
	Price       float64
	Quantity    int
	Image       string
}

// =========================
// CUSTOMER FABRIC REQUEST
// =========================

type FabricRequest struct {
	ID           int
	CustomerID   int
	CustomerName string
	Phone        string
	Image        string
	Description  string
	Quantity     int
	Status       string
	CreatedAt    string
}

// =========================
// CUSTOMER FEEDBACK
// =========================

type Feedback struct {
        ID           int
        CustomerID   int
        CustomerName string
        Rating       int
        Comment      string
        Status       string
        CreatedAt    string
}

