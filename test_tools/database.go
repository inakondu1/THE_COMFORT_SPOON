package main

import (
	"database/sql"
	"log"

	_ "github.com/mattn/go-sqlite3"
)

var db *sql.DB

func initDatabase() {
	var err error

	// Connect to database
	db, err = sql.Open("sqlite3", "asebe_fabrics.db")

	if err != nil {
		log.Fatal("Could not open database:", err)
	}

	// Test database connection
	err = db.Ping()

	if err != nil {
		log.Fatal("Could not connect to database:", err)
	}

	// =========================================================
	// PAYMENT REPORTS
	// =========================================================
	//
	// Stores a customer's claim that they have made a payment.
	// This remains PENDING until an admin checks the account
	// and confirms the payment.
	//

	_, err = db.Exec(`
        CREATE TABLE IF NOT EXISTS payment_reports (
                id INTEGER PRIMARY KEY AUTOINCREMENT,
                order_id INTEGER NOT NULL,
                customer_id INTEGER NOT NULL,
                amount REAL NOT NULL,
                status TEXT NOT NULL DEFAULT 'PENDING',
                customer_note TEXT,
                created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
                confirmed_at DATETIME,
                confirmed_by INTEGER
        )
`)

	if err != nil {
		log.Fatal("Could not create payment reports table:", err)
	}

	log.Println("Payment reports table ready")

	// =========================================================
	// PAYMENT TRANSACTIONS
	// =========================================================
	//
	// Stores separate payments made against existing orders.
	// This is NOT a new order.
	//
	// Example:
	// Previous Balance:  ₦6,000
	// Amount Paid:       ₦5,000
	// Balance Remaining: ₦1,000
	//
	// The remaining balance is what can be carried forward.

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS payments (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			order_id INTEGER NOT NULL,
			customer_id INTEGER NOT NULL,
			payment_type TEXT NOT NULL DEFAULT 'OUTSTANDING BALANCE PAYMENT',
			previous_balance REAL NOT NULL DEFAULT 0,
			amount_paid REAL NOT NULL DEFAULT 0,
			balance_remaining REAL NOT NULL DEFAULT 0,
			payment_status TEXT NOT NULL DEFAULT 'PART PAYMENT',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (order_id) REFERENCES orders(id),
			FOREIGN KEY (customer_id) REFERENCES customers(id)
		)
	`)

	if err != nil {
		log.Println("Payments table setup error:", err)
	} else {
		log.Println("Payment transactions table ready")

	}

	// =========================================================
	// PAYMENT REPORT LINK
	// =========================================================
	// Links each payment transaction to the payment report
	// that created it.

	_, err = db.Exec(`
                ALTER TABLE payments
                ADD COLUMN report_id INTEGER
        `)

	if err != nil {
		log.Println("report_id column may already exist:", err)
	} else {
		log.Println("Payment report link column added")
	}

	// Create products table
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS products (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			description TEXT,
			price REAL NOT NULL,
			quantity INTEGER NOT NULL,
			image TEXT
		)
	`)

	if err != nil {
		log.Fatal("Could not create products table:", err)
	}

	// Check if products already exist
	var count int

	err = db.QueryRow(
		"SELECT COUNT(*) FROM products",
	).Scan(&count)

	if err != nil {
		log.Fatal("Could not check products:", err)
	}

	// Add starter fabrics only when database is empty
	if count == 0 {

		_, err = db.Exec(`
			INSERT INTO products
			(name, description, price, quantity, image)
			VALUES
			(
				'Royal Ankara',
				'Beautiful premium Ankara fabric with vibrant African patterns.',
				25000,
				10,
				''
			),
			(
				'Classic Lace',
				'Elegant lace fabric suitable for weddings and special occasions.',
				35000,
				8,
				''
			),
			(
				'African Print',
				'High-quality African print fabric with beautiful traditional designs.',
				22000,
				15,
				''
			),
			(
				'Premium Senator',
				'Premium senator fabric with a smooth and luxurious finish.',
				40000,
				6,
				''
			),
			(
				'Elegant George',
				'Beautiful George fabric perfect for elegant traditional outfits.',
				55000,
				5,
				''
			),
			(
				'Luxury Silk',
				'Soft and luxurious silk fabric for beautiful and stylish outfits.',
				30000,
				12,
				''
			)
		`)

		if err != nil {
			log.Fatal("Could not add starter fabrics:", err)
		}

		log.Println("Starter fabrics added successfully")
	}

	// Create fabric requests table
	_, err = db.Exec(`
        CREATE TABLE IF NOT EXISTS fabric_requests (
                id INTEGER PRIMARY KEY AUTOINCREMENT,
                customer_id INTEGER NOT NULL,
                customer_name TEXT NOT NULL,
                phone TEXT NOT NULL,
                image TEXT,
                description TEXT NOT NULL,
                quantity INTEGER NOT NULL DEFAULT 1,
                status TEXT NOT NULL DEFAULT 'PENDING',
                created_at DATETIME DEFAULT CURRENT_TIMESTAMP
        )
        `)

	if err != nil {
		log.Fatal("Could not create fabric requests table:", err)
	}

	log.Println("Fabric requests table ready")

	// Create customers table
	_, err = db.Exec(`
    CREATE TABLE IF NOT EXISTS customers (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        full_name TEXT NOT NULL,
        phone TEXT NOT NULL UNIQUE,
        created_at DATETIME DEFAULT CURRENT_TIMESTAMP
    )
`)

	if err != nil {
		log.Fatal("Could not create customers table:", err)
	}

	log.Println("Customers table ready")

	// Add active order tracking for each customer.
	_, err = db.Exec(`
        ALTER TABLE customers
        ADD COLUMN active_order_id INTEGER
    `)

	if err != nil {
		// The column may already exist on an existing database.
		log.Println("active_order_id column may already exist:", err)
	}

	log.Println("Customer active order tracking ready")

	log.Println("ASEBE FABRICS database connected")
	log.Println("Products table ready")

	// Create orders table
	_, err = db.Exec(`
                CREATE TABLE IF NOT EXISTS orders (
                        id INTEGER PRIMARY KEY AUTOINCREMENT,
                        customer_id INTEGER NOT NULL,
                        total_amount REAL NOT NULL DEFAULT 0,
                        amount_paid REAL NOT NULL DEFAULT 0,
                        payment_status TEXT NOT NULL DEFAULT 'UNPAID',
                        last_payment_date DATETIME,
                        created_at DATETIME DEFAULT CURRENT_TIMESTAMP
                )
        `)

	if err != nil {
		log.Fatal("Could not create orders table:", err)
	}

	// Add debt tracking fields
	addDebtColumns()

	// Create order items table
	_, err = db.Exec(`
                CREATE TABLE IF NOT EXISTS order_items (
                        id INTEGER PRIMARY KEY AUTOINCREMENT,
                        order_id INTEGER NOT NULL,
                        product_id INTEGER,
                        product_name TEXT NOT NULL,
                        price REAL NOT NULL,
                        quantity INTEGER NOT NULL,
                        subtotal REAL NOT NULL
                )
        `)

	if err != nil {
		log.Fatal("Could not create order items table:", err)
	}

	log.Println("Orders table ready")
	log.Println("Order items table ready")
	// Create admin accounts table
	createAdminTable()

}

func addDebtColumns() {

	_, err := db.Exec(`
                ALTER TABLE orders
                ADD COLUMN previous_balance REAL NOT NULL DEFAULT 0
        `)

	if err != nil && err.Error() != "duplicate column name: previous_balance" {
		log.Fatal("Could not add previous_balance column:", err)
	}

	_, err = db.Exec(`
                ALTER TABLE orders
                ADD COLUMN previous_balance_order_id INTEGER
        `)

	if err != nil && err.Error() != "duplicate column name: previous_balance_order_id" {
		log.Fatal("Could not add previous_balance_order_id column:", err)
	}

	_, err = db.Exec(`
                ALTER TABLE orders
                ADD COLUMN carried_forward_to_order_id INTEGER
        `)

	if err != nil && err.Error() != "duplicate column name: carried_forward_to_order_id" {
		log.Fatal("Could not add carried_forward_to_order_id column:", err)
	}

	log.Println("Debt tracking fields ready")
}

// =========================
// ADMIN ACCOUNTS
// =========================

func createAdminTable() {

	_, err := db.Exec(`
                CREATE TABLE IF NOT EXISTS admins (
                        id INTEGER PRIMARY KEY AUTOINCREMENT,
                        username TEXT NOT NULL UNIQUE,
                        password TEXT NOT NULL,
                        created_at DATETIME DEFAULT CURRENT_TIMESTAMP
                )
        `)

	if err != nil {
		log.Fatal("Could not create admins table:", err)
	}

	log.Println("Admin table ready")

	// Create the first admin account only when none exists.
	var adminCount int

	err = db.QueryRow(
		"SELECT COUNT(*) FROM admins",
	).Scan(&adminCount)

	if err != nil {
		log.Fatal("Could not check admin accounts:", err)
	}

	if adminCount == 0 {

		_, err = db.Exec(`
			INSERT INTO admins
			(username, password)
			VALUES (?, ?)
		`,
			"admin",
			"admin123",
		)

		if err != nil {
			log.Fatal("Could not create first admin account:", err)
		}

		log.Println("Default admin account created")
	}
}

func inspectPaymentSchema() {
	log.Println("========== DATABASE SCHEMA CHECK ==========")

	rows, err := db.Query(`
                SELECT name, sql
                FROM sqlite_master
                WHERE type = 'table'
                  AND name IN ('orders', 'payments')
                ORDER BY name
        `)

	if err != nil {
		log.Println("Schema check error:", err)
		return
	}

	defer rows.Close()

	for rows.Next() {
		var tableName string
		var tableSQL string

		if err := rows.Scan(&tableName, &tableSQL); err != nil {
			log.Println("Schema read error:", err)
			return
		}

		log.Println("TABLE:", tableName)
		log.Println(tableSQL)
	}

	if err := rows.Err(); err != nil {
		log.Println("Schema rows error:", err)
	}

	log.Println("===========================================")
}
