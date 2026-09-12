package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// =========================
// DATE FORMATTER
// =========================

func formatNigeriaDate(value string) string {

	if value == "" {
		return ""
	}

	layouts := []string{
		time.RFC3339,
		"2006-01-02 15:04:05",
	}

	for _, layout := range layouts {

		parsed, err := time.Parse(layout, value)

		if err == nil {
			return parsed.In(time.FixedZone("WAT", 60*60)).
				Format("02 January 2006, 03:04 PM")
		}
	}

	return value
}

// =========================
// CUSTOMER SESSION HELPERS
// =========================

func setCustomerSession(w http.ResponseWriter, customerID int) {

	http.SetCookie(w, &http.Cookie{
		Name:     "comfort_spoon_customer",
		Value:    strconv.Itoa(customerID),
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func getCustomerSession(r *http.Request) (int, bool) {

	cookie, err := r.Cookie("comfort_spoon_customer")

	if err != nil {
		return 0, false
	}

	customerID, err := strconv.Atoi(cookie.Value)

	if err != nil || customerID <= 0 {
		return 0, false
	}

	return customerID, true
}

// =========================
// CUSTOMER NAVIGATION DATA
// =========================

type NavData struct {
	LoggedIn     bool
	CustomerName string
	Products     []Product
}

func getNavData(r *http.Request) NavData {

	data := NavData{}

	customerID, ok := getCustomerSession(r)

	if ok {

		var customerName string

		err := db.QueryRow(`
                        SELECT full_name
                        FROM customers
                        WHERE id = ?
                `, customerID).Scan(&customerName)

		if err == nil {
			data.LoggedIn = true
			data.CustomerName = customerName
		}
	}

	return data
}

func renderTemplate(w http.ResponseWriter, filename string, data interface{}) {
	tmpl, err := template.New(filepath.Base(filename)).Funcs(template.FuncMap{
		"subtract": func(a, b float64) float64 {
			return a - b
		},
	}).ParseFiles(filename)
	if err != nil {
		http.Error(w, "Template error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	err = tmpl.Execute(w, data)
	if err != nil {
		http.Error(w, "Page error: "+err.Error(), http.StatusInternalServerError)
		return
	}
}

// =========================
// PHONE VALIDATION
// =========================

func validPhone(phone string) bool {

	if len(phone) != 11 {
		return false
	}

	for _, c := range phone {

		if c < '0' || c > '9' {
			return false
		}
	}

	return true
}

// =========================
// HOME
// =========================

func homeHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	type HomePage struct {
		LoggedIn     bool
		CustomerName string
		Products     []Product
	}

	data := HomePage{}

	rows, err := db.Query(`
                SELECT id, name, description, price, quantity, image
                FROM products
                ORDER BY id DESC
                LIMIT 4
        `)
	if err != nil {
		http.Error(w, "Could not load menu: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var product Product

		err := rows.Scan(
			&product.ID,
			&product.Name,
			&product.Description,
			&product.Price,
			&product.Quantity,
			&product.Image,
		)
		if err != nil {
			http.Error(w, "Could not load menu: "+err.Error(), http.StatusInternalServerError)
			return
		}

		data.Products = append(data.Products, product)
	}

	customerID, ok := getCustomerSession(r)

	if ok {
		var customerName string

		err := db.QueryRow(`
			SELECT full_name
			FROM customers
			WHERE id = ?
		`, customerID).Scan(&customerName)

		if err == nil {
			data.LoggedIn = true
			data.CustomerName = customerName
		}
	}

	renderTemplate(w, "templates/home.html", data)
}

// =========================
// SHOP
// =========================

func shopHandler(w http.ResponseWriter, r *http.Request) {
	search := strings.TrimSpace(r.URL.Query().Get("search"))

	rows, err := db.Query(`
                SELECT id, name, description, price, quantity, image
                FROM products
                WHERE name LIKE ? OR description LIKE ?
                ORDER BY CASE WHEN name LIKE ? THEN 0 WHEN description LIKE ? THEN 1 ELSE 2 END, id DESC
        `, "%"+search+"%", "%"+search+"%", "%"+search+"%", "%"+search+"%")
	if err != nil {
		http.Error(w, "Could not load fabrics: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var products []Product

	for rows.Next() {
		var product Product

		err := rows.Scan(
			&product.ID,
			&product.Name,
			&product.Description,
			&product.Price,
			&product.Quantity,
			&product.Image,
		)

		if err != nil {
			http.Error(w, "Could not read fabrics: "+err.Error(), http.StatusInternalServerError)
			return
		}

		products = append(products, product)

	}

	if err := rows.Err(); err != nil {
		http.Error(w, "Could not read fabrics: "+err.Error(), http.StatusInternalServerError)
		return
	}

	type ShopPage struct {
		Products     []Product
		NewArrivals  []Product
		IsAdmin      bool
		LoggedIn     bool
		CustomerName string
	}

	var newArrivals []Product

	arrivalRows, err := db.Query(`
                SELECT id, name, description, price, quantity, image
                FROM products
                ORDER BY id DESC
                LIMIT 4
        `)
	if err == nil {
		defer arrivalRows.Close()
		for arrivalRows.Next() {
			var product Product
			if arrivalRows.Scan(&product.ID, &product.Name, &product.Description, &product.Price, &product.Quantity, &product.Image) == nil {
				newArrivals = append(newArrivals, product)
			}
		}
	}

	_, isAdmin := getAdminSession(r)

	data := ShopPage{
		Products:    products,
		NewArrivals: newArrivals,
		IsAdmin:     isAdmin,
	}

	customerID, ok := getCustomerSession(r)

	if ok {

		var customerName string

		err := db.QueryRow(`
				SELECT full_name
				FROM customers
				WHERE id = ?
			`, customerID).Scan(&customerName)

		if err == nil {
			data.LoggedIn = true
			data.CustomerName = customerName
		}
	}

	renderTemplate(w, "templates/shop.html", data)
}

// =========================
// PRODUCT DETAILS
// =========================

func productHandler(w http.ResponseWriter, r *http.Request) {

	id, err := strconv.Atoi(r.URL.Query().Get("id"))

	if err != nil || id <= 0 {
		http.Error(w, "Invalid product", http.StatusBadRequest)
		return
	}

	var product Product

	err = db.QueryRow(`
                        SELECT id, name, description, price, quantity, image
                        FROM products
                        WHERE id = ?
                `, id).Scan(
		&product.ID,
		&product.Name,
		&product.Description,
		&product.Price,
		&product.Quantity,
		&product.Image,
	)

	if err == sql.ErrNoRows {
		http.Error(w, "Product not found", http.StatusNotFound)
		return
	}

	if err != nil {
		http.Error(w, "Unable to load product", http.StatusInternalServerError)
		return
	}

	renderTemplate(w, "templates/product.html", product)
}

// =========================
// CART
// =========================

func cartHandler(w http.ResponseWriter, r *http.Request) {
	renderTemplate(w, "templates/cart.html", nil)
}

// =========================
// PAYMENT
// =========================

func paymentHandler(w http.ResponseWriter, r *http.Request) {

	if r.Method != http.MethodGet {
		http.Error(
			w,
			"Method not allowed",
			http.StatusMethodNotAllowed,
		)
		return
	}

	customerIDText := r.URL.Query().Get("customer")
	name := r.URL.Query().Get("name")
	phone := r.URL.Query().Get("phone")
	address := r.URL.Query().Get("address")
	note := r.URL.Query().Get("note")
	cartJSON := r.URL.Query().Get("cart")

	// PAYMENT from the navigation menu.
	//
	// If the customer is not logged in, send them to LOGIN.
	// If they are logged in but have no pending order,
	// send them to the cart instead of showing an error page.
	if customerIDText == "" || phone == "" || cartJSON == "" {

		_, loggedIn := getCustomerSession(r)

		if !loggedIn {
			http.Redirect(
				w,
				r,
				"/login?message="+url.QueryEscape(
					"Please log in before making a payment.",
				),
				http.StatusSeeOther,
			)
			return
		}

		http.Redirect(
			w,
			r,
			"/cart?message="+url.QueryEscape(
				"You are logged in. Please add your fabrics to the cart before making a payment.",
			),
			http.StatusSeeOther,
		)
		return
	}

	customerID, err := strconv.Atoi(customerIDText)

	if err != nil || customerID <= 0 {
		http.Error(
			w,
			"Invalid customer",
			http.StatusBadRequest,
		)
		return
	}

	// Make sure the customer is still registered.
	var registeredCustomerID int

	err = db.QueryRow(`
                SELECT id
                FROM customers
                WHERE id = ?
                  AND phone = ?
        `, customerID, phone).Scan(&registeredCustomerID)

	if err == sql.ErrNoRows {
		http.Error(
			w,
			"Customer account could not be verified.",
			http.StatusBadRequest,
		)
		return
	}

	if err != nil {
		http.Error(
			w,
			"Could not verify customer: "+err.Error(),
			http.StatusInternalServerError,
		)
		return
	}

	// Read the cart without creating an order.
	var cart []struct {
		ID       int     `json:"id"`
		Name     string  `json:"name"`
		Price    float64 `json:"price"`
		Quantity int     `json:"quantity"`
	}

	err = json.Unmarshal([]byte(cartJSON), &cart)

	if err != nil || len(cart) == 0 {
		http.Error(
			w,
			"Could not read your cart.",
			http.StatusBadRequest,
		)
		return
	}

	// Calculate the current fabric total.
	var currentOrderTotal float64

	for _, item := range cart {

		if item.Quantity <= 0 || item.Price < 0 {
			http.Error(
				w,
				"Invalid item in cart",
				http.StatusBadRequest,
			)
			return
		}

		currentOrderTotal +=
			item.Price * float64(item.Quantity)
	}

	if currentOrderTotal <= 0 {
		http.Error(
			w,
			"Order total must be greater than zero",
			http.StatusBadRequest,
		)
		return
	}

	// Find any unpaid balance from a previous order.
	var previousBalance float64
	var previousBalanceOrderID int64

	err = db.QueryRow(`
                SELECT
                        total_amount - amount_paid,
                        id
                FROM orders
                WHERE customer_id = ?
                  AND amount_paid < total_amount
                  AND carried_forward_to_order_id IS NULL
                ORDER BY id DESC
                LIMIT 1
        `,
		customerID,
	).Scan(
		&previousBalance,
		&previousBalanceOrderID,
	)

	if err == sql.ErrNoRows {
		previousBalance = 0
		previousBalanceOrderID = 0
	} else if err != nil {
		http.Error(
			w,
			"Could not check previous balance: "+err.Error(),
			http.StatusInternalServerError,
		)
		return
	}

	total := currentOrderTotal + previousBalance

	type PaymentPage struct {
		CustomerID             int
		Name                   string
		Phone                  string
		Address                string
		Note                   string
		Cart                   string
		CurrentOrderTotal      float64
		PreviousBalance        float64
		PreviousBalanceOrderID int64
		Total                  float64
		AmountPaid             float64
		Balance                float64
	}

	data := PaymentPage{
		CustomerID:             customerID,
		Name:                   name,
		Phone:                  phone,
		Address:                address,
		Note:                   note,
		Cart:                   cartJSON,
		CurrentOrderTotal:      currentOrderTotal,
		PreviousBalance:        previousBalance,
		PreviousBalanceOrderID: previousBalanceOrderID,
		Total:                  total,
		AmountPaid:             0,
		Balance:                total,
	}

	renderTemplate(
		w,
		"templates/payment.html",
		data,
	)
}

func flutterwavePayHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	secretKey := strings.TrimSpace(os.Getenv("FLW_SECRET_KEY"))
	if secretKey == "" {
		http.Error(w, "Flutterwave is not configured.", http.StatusInternalServerError)
		return
	}

	customerID, ok := getCustomerSession(r)
	if !ok {
		http.Error(w, "Please log in before making a payment.", http.StatusUnauthorized)
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	phone := strings.TrimSpace(r.FormValue("phone"))
	address := strings.TrimSpace(r.FormValue("address"))
	note := strings.TrimSpace(r.FormValue("note"))
	cartJSON := strings.TrimSpace(r.FormValue("cart"))

	if cartJSON == "" {
		http.Error(w, "Your cart is empty.", http.StatusBadRequest)
		return
	}

	var cart []struct {
		ID       int     `json:"id"`
		Name     string  `json:"name"`
		Price    float64 `json:"price"`
		Quantity int     `json:"quantity"`
	}

	log.Printf("FLUTTERWAVE CART RECEIVED: %s", cartJSON)
	if err := json.Unmarshal([]byte(cartJSON), &cart); err != nil {
		http.Error(w, "Invalid cart data.", http.StatusBadRequest)
		return
	}

	currentOrderTotal := 0.0

	var err error

	for _, item := range cart {
		var price float64
		var stock int

		err := db.QueryRow(
			"SELECT price, quantity FROM products WHERE id = ?",
			item.ID,
		).Scan(&price, &stock)

		if err != nil {
			http.Error(w, "Unable to verify your cart.", http.StatusBadRequest)
			return
		}

		if item.Quantity <= 0 || item.Quantity > stock {
			http.Error(w, "One or more products are no longer available in the requested quantity.", http.StatusBadRequest)
			return
		}

		currentOrderTotal += price * float64(item.Quantity)
	}

	var previousBalance float64
	var previousOrderID sql.NullInt64

	err = db.QueryRow(`
		SELECT
			COALESCE(total_amount - amount_paid, 0),
			id
		FROM orders
		WHERE customer_id = ?
		  AND amount_paid < total_amount
		ORDER BY id DESC
		LIMIT 1
	`, customerID).Scan(&previousBalance, &previousOrderID)

	if err == sql.ErrNoRows {
		previousBalance = 0
		previousOrderID = sql.NullInt64{}
	} else if err != nil {
		http.Error(w, "Unable to check your outstanding balance.", http.StatusInternalServerError)
		return
	}

	total := currentOrderTotal + previousBalance

	transactionReference := fmt.Sprintf(
		"TCS-%d-%d",
		time.Now().UnixNano(),
		customerID,
	)

	result, err := db.Exec(`
		INSERT INTO flutterwave_pending_payments (
			customer_id,
			name,
			phone,
			address,
			note,
			cart_json,
			current_order_total,
			previous_balance,
			previous_balance_order_id,
			total_amount,
			amount_paid,
			transaction_reference,
			status
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'PENDING')
	`,
		customerID,
		name,
		phone,
		address,
		note,
		cartJSON,
		currentOrderTotal,
		previousBalance,
		previousOrderID,
		total,
		total,
		transactionReference,
	)

	if err != nil {
		http.Error(w, "Unable to prepare your Flutterwave payment: "+err.Error(), http.StatusInternalServerError)
		return
	}

	pendingID, err := result.LastInsertId()
	if err != nil {
		http.Error(w, "Unable to prepare your payment.", http.StatusInternalServerError)
		return
	}

	callbackURL := strings.TrimRight(
		strings.TrimSpace(os.Getenv("PUBLIC_BASE_URL")),
		"/",
	)

	if callbackURL == "" {
		callbackURL = "http://localhost:" + getPort()
	}

	callbackURL += "/flutterwave/callback"

	payload := map[string]interface{}{
		"tx_ref":       transactionReference,
		"amount":       total,
		"currency":     "NGN",
		"redirect_url": callbackURL,
		"customer": map[string]string{
			"email":       "customer@example.com",
			"name":        name,
			"phonenumber": phone,
		},
		"customizations": map[string]string{
			"title":       "The Comfort Spoon",
			"description": "Restaurant order payment",
		},
		"meta": map[string]interface{}{
			"pending_payment_id": pendingID,
			"customer_id":        customerID,
		},
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		http.Error(w, "Unable to prepare Flutterwave payment.", http.StatusInternalServerError)
		return
	}

	req, err := http.NewRequest(
		http.MethodPost,
		"https://api.flutterwave.com/v3/payments",
		bytes.NewBuffer(payloadBytes),
	)
	if err != nil {
		http.Error(w, "Unable to connect to Flutterwave.", http.StatusBadGateway)
		return
	}

	req.Header.Set("Authorization", "Bearer "+secretKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}

	response, err := client.Do(req)
	if err != nil {
		http.Error(w, "Unable to connect to Flutterwave.", http.StatusBadGateway)
		return
	}
	defer response.Body.Close()

	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		http.Error(w, "Unable to read Flutterwave response.", http.StatusBadGateway)
		return
	}

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		log.Printf(
			"Flutterwave API error: HTTP %d: %s",
			response.StatusCode,
			string(responseBody),
		)

		http.Error(
			w,
			"Flutterwave could not create the payment.",
			http.StatusBadGateway,
		)
		return
	}

	var flutterwaveResponse struct {
		Status string `json:"status"`
		Data   struct {
			Link string `json:"link"`
		} `json:"data"`
	}

	if err := json.Unmarshal(responseBody, &flutterwaveResponse); err != nil {
		http.Error(w, "Invalid Flutterwave response.", http.StatusBadGateway)
		return
	}

	if flutterwaveResponse.Status != "success" || flutterwaveResponse.Data.Link == "" {
		http.Error(w, "Flutterwave did not return a payment link.", http.StatusBadGateway)
		return
	}

	http.Redirect(
		w,
		r,
		flutterwaveResponse.Data.Link,
		http.StatusFound,
	)
}

func checkoutHandler(w http.ResponseWriter, r *http.Request) {

	// =========================
	// CHECKOUT PAGE
	// =========================
	if r.Method == http.MethodGet {

		customerID, loggedIn := getCustomerSession(r)

		if !loggedIn || customerID <= 0 {
			http.Redirect(
				w,
				r,
				"/login?message="+url.QueryEscape(
					"Please log in before checkout.",
				),
				http.StatusSeeOther,
			)
			return
		}

		var customer struct {
			ID    int
			Name  string
			Phone string
		}

		err := db.QueryRow(`
                        SELECT id, full_name, phone
                        FROM customers
                        WHERE id = ?
                `, customerID).Scan(
			&customer.ID,
			&customer.Name,
			&customer.Phone,
		)

		if err == sql.ErrNoRows {
			http.Redirect(
				w,
				r,
				"/login?message="+url.QueryEscape(
					"Customer account could not be found.",
				),
				http.StatusSeeOther,
			)
			return
		}

		if err != nil {
			http.Error(
				w,
				"Could not load customer: "+err.Error(),
				http.StatusInternalServerError,
			)
			return
		}

		data := struct {
			CustomerID int
			Name       string
			Phone      string
		}{
			CustomerID: customer.ID,
			Name:       customer.Name,
			Phone:      customer.Phone,
		}

		renderTemplate(
			w,
			"templates/checkout.html",
			data,
		)
		return
	}

	if r.Method != http.MethodPost {
		http.Error(
			w,
			"Method not allowed",
			http.StatusMethodNotAllowed,
		)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(
			w,
			"Could not process payment",
			http.StatusBadRequest,
		)
		return
	}

	customerID, err := strconv.Atoi(r.FormValue("customer"))
	if err != nil || customerID <= 0 {
		http.Error(w, "Invalid customer", http.StatusBadRequest)
		return
	}

	name := r.FormValue("name")
	phone := r.FormValue("phone")
	address := r.FormValue("address")
	note := r.FormValue("note")
	cartJSON := r.FormValue("cart")
	amountPaidText := r.FormValue("amount_paid")
	paymentConfirmed := r.FormValue("payment_confirmed")

	if paymentConfirmed != "yes" {
		http.Error(
			w,
			"Please confirm that payment has been made.",
			http.StatusBadRequest,
		)
		return
	}

	amountPaid, err := strconv.ParseFloat(amountPaidText, 64)
	if err != nil || amountPaid <= 0 {
		http.Error(
			w,
			"Invalid payment amount",
			http.StatusBadRequest,
		)
		return
	}

	var cart []struct {
		ID       int     `json:"id"`
		Name     string  `json:"name"`
		Price    float64 `json:"price"`
		Quantity int     `json:"quantity"`
	}

	if err := json.Unmarshal([]byte(cartJSON), &cart); err != nil || len(cart) == 0 {
		http.Error(
			w,
			"Could not read your cart",
			http.StatusBadRequest,
		)
		return
	}

	// Verify the registered customer.
	var registeredPhone string

	err = db.QueryRow(`
                SELECT phone
                FROM customers
                WHERE id = ?
        `, customerID).Scan(&registeredPhone)

	if err == sql.ErrNoRows {
		http.Error(w, "Customer not found", http.StatusBadRequest)
		return
	}

	if err != nil {
		http.Error(
			w,
			"Could not verify customer: "+err.Error(),
			http.StatusInternalServerError,
		)
		return
	}

	if registeredPhone != phone {
		http.Error(
			w,
			"Customer verification failed",
			http.StatusBadRequest,
		)
		return
	}

	// Calculate the new fabrics total.
	var newFabricTotal float64

	for _, item := range cart {

		if item.Quantity <= 0 || item.Price < 0 {
			http.Error(
				w,
				"Invalid item in cart",
				http.StatusBadRequest,
			)
			return
		}

		newFabricTotal += item.Price * float64(item.Quantity)
	}

	if newFabricTotal <= 0 {
		http.Error(
			w,
			"Order total must be greater than zero",
			http.StatusBadRequest,
		)
		return
	}

	// Find an unpaid balance from a previous order.
	var previousBalance float64
	var previousBalanceOrderID int64

	err = db.QueryRow(`
                SELECT
                        total_amount - amount_paid,
                        id
                FROM orders
                WHERE customer_id = ?
                  AND amount_paid < total_amount
                  AND carried_forward_to_order_id IS NULL
                ORDER BY id DESC
                LIMIT 1
        `,
		customerID,
	).Scan(
		&previousBalance,
		&previousBalanceOrderID,
	)

	if err == sql.ErrNoRows {
		previousBalance = 0
		previousBalanceOrderID = 0
	} else if err != nil {
		http.Error(
			w,
			"Could not check previous balance: "+err.Error(),
			http.StatusInternalServerError,
		)
		return
	}

	total := newFabricTotal + previousBalance

	// Payment cannot exceed the amount currently due.
	if amountPaid > total {
		http.Error(
			w,
			fmt.Sprintf(
				"Payment cannot be greater than the total of ₦%.2f",
				total,
			),
			http.StatusBadRequest,
		)
		return
	}

	balanceRemaining := total - amountPaid

	if balanceRemaining < 0 {
		balanceRemaining = 0
	}

	// =========================================================
	// CREATE THE ORDER ONLY NOW
	// =========================================================

	result, err := db.Exec(`
                INSERT INTO orders
                (
                        customer_id,
                        total_amount,
                        amount_paid,
                        payment_status,
                        previous_balance,
                        previous_balance_order_id,
                        last_payment_date
                )
                VALUES (?, ?, ?, ?, ?, ?, NULL)
        `,
		customerID,
		total,
		0,
		"PART PAYMENT",
		previousBalance,
		previousBalanceOrderID,
	)

	if err != nil {
		http.Error(
			w,
			"Could not create order: "+err.Error(),
			http.StatusInternalServerError,
		)
		return
	}

	orderID, err := result.LastInsertId()

	if err != nil {
		http.Error(
			w,
			"Could not get order ID: "+err.Error(),
			http.StatusInternalServerError,
		)
		return
	}

	// Save this as the customer's active order.
	_, err = db.Exec(`
                UPDATE customers
                SET active_order_id = ?
                WHERE id = ?
        `,
		orderID,
		customerID,
	)

	if err != nil {
		http.Error(
			w,
			"Could not save active order: "+err.Error(),
			http.StatusInternalServerError,
		)
		return
	}

	// Link the previous unpaid order to this new order.
	if previousBalanceOrderID > 0 {

		_, err = db.Exec(`
                        UPDATE orders
                        SET carried_forward_to_order_id = ?
                        WHERE id = ?
                `,
			orderID,
			previousBalanceOrderID,
		)

		if err != nil {
			http.Error(
				w,
				"Could not link previous balance: "+err.Error(),
				http.StatusInternalServerError,
			)
			return
		}
	}

	// Save the fabrics in the new order.
	for _, item := range cart {

		subtotal := item.Price * float64(item.Quantity)

		_, err = db.Exec(`
                        INSERT INTO order_items
                        (
                                order_id,
                                product_id,
                                product_name,
                                price,
                                quantity,
                                subtotal
                        )
                        VALUES (?, ?, ?, ?, ?, ?)
                `,
			orderID,
			item.ID,
			item.Name,
			item.Price,
			item.Quantity,
			subtotal,
		)

		if err != nil {
			http.Error(
				w,
				"Could not save order item: "+err.Error(),
				http.StatusInternalServerError,
			)
			return
		}
		// Reduce available stock by the quantity ordered.
		result, err = db.Exec(`
                        UPDATE products
                        SET quantity = quantity - ?
                        WHERE name = ?
                          AND quantity >= ?
                `,
			item.Quantity,
			item.Name,
			item.Quantity,
		)

		if err != nil {
			http.Error(
				w,
				"Could not update stock for "+item.Name+": "+err.Error(),
				http.StatusInternalServerError,
			)
			return
		}

		rowsAffected, err := result.RowsAffected()

		if err != nil {
			http.Error(
				w,
				"Could not verify stock update for "+item.Name+": "+err.Error(),
				http.StatusInternalServerError,
			)
			return
		}

		if rowsAffected == 0 {
			http.Error(
				w,
				"Could not reduce stock for "+item.Name+". The fabric may no longer have enough stock.",
				http.StatusBadRequest,
			)
			return
		}

	}

	// =========================================================
	// NOTIFY ADMIN THAT CUSTOMER HAS REPORTED A PAYMENT
	// =========================================================
	//
	// Customer says payment has been made.
	// Admin must still check the account and confirm it.
	//

	_, err = db.Exec(`
                INSERT INTO payment_reports
                (
                        order_id,
                        customer_id,
                        amount,
                        status,
                        customer_note
                )
                VALUES (?, ?, ?, 'PENDING', ?)
        `,
		orderID,
		customerID,
		amountPaid,
		"Customer reported that payment has been made.",
	)

	if err != nil {
		http.Error(
			w,
			"Could not create payment notification: "+err.Error(),
			http.StatusInternalServerError,
		)
		return
	}

	log.Println("========== PAYMENT REPORT CREATED ==========")
	log.Println("Order ID:", orderID)
	log.Println("Customer ID:", customerID)
	log.Println("Amount:", amountPaid)
	log.Println("Status: PENDING")
	log.Println("============================================")

	log.Println("========== ORDER CREATED AFTER PAYMENT ==========")
	log.Println("Order ID:", orderID)
	log.Println("Customer:", name)
	log.Println("Phone:", phone)
	log.Println("Address:", address)
	log.Println("Note:", note)
	log.Println("New fabrics:", newFabricTotal)
	log.Println("Previous balance:", previousBalance)
	log.Println("Total:", total)
	log.Println("Amount paid:", amountPaid)
	log.Println("Balance:", balanceRemaining)
	log.Println("==================================================")

	http.Redirect(
		w,
		r,
		"/receipt?order="+url.QueryEscape(strconv.FormatInt(orderID, 10)),
		http.StatusSeeOther,
	)
}

// =========================
// ADMIN PAYMENT REPORTS
// =========================

func flutterwaveCallbackHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	secretKey := strings.TrimSpace(os.Getenv("FLW_SECRET_KEY"))
	if secretKey == "" {
		http.Error(w, "Flutterwave is not configured.", http.StatusInternalServerError)
		return
	}

	transactionID := strings.TrimSpace(r.URL.Query().Get("transaction_id"))
	transactionReference := strings.TrimSpace(r.URL.Query().Get("tx_ref"))
	status := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("status")))

	if status != "successful" || transactionID == "" || transactionReference == "" {
		http.Error(w, "Flutterwave payment was not completed.", http.StatusBadRequest)
		return
	}

	var pending struct {
		ID                int64
		CustomerID        int
		Name              string
		Phone             string
		Address           string
		Note              string
		CartJSON          string
		CurrentOrderTotal float64
		PreviousBalance   float64
		PreviousOrderID   sql.NullInt64
		TotalAmount       float64
		AmountPaid        float64
		Status            string
	}

	err := db.QueryRow(`
		SELECT
			id,
			customer_id,
			name,
			phone,
			address,
			note,
			cart_json,
			current_order_total,
			previous_balance,
			previous_balance_order_id,
			total_amount,
			amount_paid,
			status
		FROM flutterwave_pending_payments
		WHERE transaction_reference = ?
	`, transactionReference).Scan(
		&pending.ID,
		&pending.CustomerID,
		&pending.Name,
		&pending.Phone,
		&pending.Address,
		&pending.Note,
		&pending.CartJSON,
		&pending.CurrentOrderTotal,
		&pending.PreviousBalance,
		&pending.PreviousOrderID,
		&pending.TotalAmount,
		&pending.AmountPaid,
		&pending.Status,
	)

	if err == sql.ErrNoRows {
		http.Error(w, "Payment record not found.", http.StatusNotFound)
		return
	}

	if err != nil {
		http.Error(w, "Unable to load payment record.", http.StatusInternalServerError)
		return
	}

	if pending.Status == "SUCCESS" {
		http.Error(w, "This payment has already been processed.", http.StatusConflict)
		return
	}

	verifyURL := "https://api.flutterwave.com/v3/transactions/" +
		url.PathEscape(transactionID) +
		"/verify"

	req, err := http.NewRequest(http.MethodGet, verifyURL, nil)
	if err != nil {
		http.Error(w, "Unable to verify payment.", http.StatusBadGateway)
		return
	}

	req.Header.Set("Authorization", "Bearer "+secretKey)

	client := &http.Client{Timeout: 30 * time.Second}

	response, err := client.Do(req)
	if err != nil {
		http.Error(w, "Unable to connect to Flutterwave for verification.", http.StatusBadGateway)
		return
	}
	defer response.Body.Close()

	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		http.Error(w, "Unable to read Flutterwave verification.", http.StatusBadGateway)
		return
	}

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		log.Printf(
			"Flutterwave verification error: HTTP %d: %s",
			response.StatusCode,
			string(responseBody),
		)
		http.Error(w, "Flutterwave could not verify the payment.", http.StatusBadGateway)
		return
	}

	var verification struct {
		Status string `json:"status"`
		Data   struct {
			Status   string  `json:"status"`
			TxRef    string  `json:"tx_ref"`
			Amount   float64 `json:"amount"`
			Currency string  `json:"currency"`
		} `json:"data"`
	}

	if err := json.Unmarshal(responseBody, &verification); err != nil {
		http.Error(w, "Invalid Flutterwave verification response.", http.StatusBadGateway)
		return
	}

	if verification.Status != "success" ||
		strings.ToLower(verification.Data.Status) != "successful" {
		http.Error(w, "Flutterwave payment verification failed.", http.StatusBadRequest)
		return
	}

	if verification.Data.TxRef != transactionReference {
		http.Error(w, "Payment reference verification failed.", http.StatusBadRequest)
		return
	}

	if strings.ToUpper(verification.Data.Currency) != "NGN" {
		http.Error(w, "Payment currency verification failed.", http.StatusBadRequest)
		return
	}

	if verification.Data.Amount != pending.TotalAmount {
		http.Error(w, "Payment amount verification failed.", http.StatusBadRequest)
		return
	}

	var existingPayment int

	err = db.QueryRow(
		"SELECT COUNT(*) FROM payments WHERE transaction_reference = ?",
		transactionReference,
	).Scan(&existingPayment)

	if err != nil {
		http.Error(w, "Unable to check payment record.", http.StatusInternalServerError)
		return
	}

	if existingPayment > 0 {
		http.Error(w, "This payment has already been recorded.", http.StatusConflict)
		return
	}

	var cart []struct {
		ProductID int `json:"product_id"`
		Quantity  int `json:"quantity"`
	}

	if err := json.Unmarshal([]byte(pending.CartJSON), &cart); err != nil || len(cart) == 0 {
		http.Error(w, "Unable to restore your order.", http.StatusBadRequest)
		return
	}

	tx, err := db.Begin()
	if err != nil {
		http.Error(w, "Unable to complete your order.", http.StatusInternalServerError)
		return
	}

	defer tx.Rollback()

	var orderID int64

	result, err := tx.Exec(`
		INSERT INTO orders (
			customer_id,
			customer_name,
			phone,
			address,
			note,
			total_amount,
			amount_paid,
			payment_status,
			order_date,
			previous_balance,
			previous_order_id
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, ?, ?)
	`,
		pending.CustomerID,
		pending.Name,
		pending.Phone,
		pending.Address,
		pending.Note,
		pending.TotalAmount,
		pending.AmountPaid,
		"PAID",
		pending.PreviousBalance,
		pending.PreviousOrderID,
	)

	if err != nil {
		http.Error(w, "Unable to create your order.", http.StatusInternalServerError)
		return
	}

	orderID, err = result.LastInsertId()
	if err != nil {
		http.Error(w, "Unable to create your order.", http.StatusInternalServerError)
		return
	}

	_, err = tx.Exec(`
		UPDATE customers
		SET active_order_id = ?
		WHERE id = ?
	`, orderID, pending.CustomerID)

	if err != nil {
		http.Error(w, "Unable to update customer order.", http.StatusInternalServerError)
		return
	}

	if pending.PreviousOrderID.Valid {
		_, err = tx.Exec(`
			UPDATE orders
			SET previous_order_id = ?
			WHERE id = ?
		`, pending.PreviousOrderID.Int64, orderID)

		if err != nil {
			http.Error(w, "Unable to link previous order.", http.StatusInternalServerError)
			return
		}
	}

	for _, item := range cart {
		var price float64

		err = tx.QueryRow(
			"SELECT price FROM products WHERE id = ?",
			item.ProductID,
		).Scan(&price)

		if err != nil {
			http.Error(w, "Unable to load product.", http.StatusInternalServerError)
			return
		}

		_, err = tx.Exec(`
			INSERT INTO order_items (
				order_id,
				product_id,
				quantity,
				price
			)
			VALUES (?, ?, ?, ?)
		`,
			orderID,
			item.ProductID,
			item.Quantity,
			price,
		)

		if err != nil {
			http.Error(w, "Unable to save order item.", http.StatusInternalServerError)
			return
		}

		result, err = tx.Exec(`
			UPDATE products
			SET quantity = quantity - ?
			WHERE id = ?
			  AND quantity >= ?
		`,
			item.Quantity,
			item.ProductID,
			item.Quantity,
		)

		if err != nil {
			http.Error(w, "Unable to update product stock.", http.StatusInternalServerError)
			return
		}

		rowsAffected, err := result.RowsAffected()
		if err != nil || rowsAffected == 0 {
			http.Error(w, "Product stock is no longer available.", http.StatusBadRequest)
			return
		}
	}

	_, err = tx.Exec(`
		INSERT INTO payments (
			order_id,
			customer_id,
			amount,
			payment_type,
			payment_date,
			previous_balance,
			amount_paid,
			balance_remaining,
			payment_status,
			report_id,
			payment_method,
			transaction_reference,
			flutterwave_transaction_id,
			flutterwave_status
		)
		VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP, ?, ?, ?, ?, NULL, ?, ?, ?, ?)
	`,
		orderID,
		pending.CustomerID,
		pending.AmountPaid,
		"FLUTTERWAVE PAYMENT",
		pending.PreviousBalance,
		pending.AmountPaid,
		0,
		"PAID",
		"FLUTTERWAVE",
		transactionReference,
		transactionID,
		"successful",
	)

	if err != nil {
		http.Error(w, "Unable to record payment.", http.StatusInternalServerError)
		return
	}

	_, err = tx.Exec(`
		UPDATE flutterwave_pending_payments
		SET
			status = 'SUCCESS',
			flutterwave_transaction_id = ?
		WHERE id = ?
	`,
		transactionID,
		pending.ID,
	)

	if err != nil {
		http.Error(w, "Unable to finalize payment.", http.StatusInternalServerError)
		return
	}

	if err := tx.Commit(); err != nil {
		http.Error(w, "Unable to complete your order.", http.StatusInternalServerError)
		return
	}

	http.Redirect(
		w,
		r,
		fmt.Sprintf("/receipt?order=%d", orderID),
		http.StatusFound,
	)
}

func formatNigeriaTime(value string) string {
	parsed, err := time.Parse("2006-01-02T15:04:05Z", value)
	if err != nil {
		return value
	}

	nigeria, err := time.LoadLocation("Africa/Lagos")
	if err != nil {
		return value
	}

	return parsed.In(nigeria).Format("02 Jan 2006, 03:04 PM")
}

func adminPaymentReportsHandler(w http.ResponseWriter, r *http.Request) {

	_, loggedIn := getAdminSession(r)

	if !loggedIn {
		http.Redirect(
			w,
			r,
			"/comfort-spoon-control/login",
			http.StatusSeeOther,
		)
		return
	}

	rows, err := db.Query(`
		SELECT
			pr.id,
			pr.order_id,
			pr.customer_id,
			c.full_name,
			c.phone,
			pr.amount,
			pr.status,
			pr.customer_note,
			pr.created_at
		FROM payment_reports pr
		JOIN customers c ON c.id = pr.customer_id
		ORDER BY pr.id DESC
	`)

	if err != nil {
		http.Error(
			w,
			"Could not load payment reports: "+err.Error(),
			http.StatusInternalServerError,
		)
		return
	}

	defer rows.Close()

	fmt.Fprintln(w, "<html><body>")
	fmt.Fprintln(w, `<div style="text-align:right; margin:20px;"><a href="/admin" style="display:inline-block; padding:12px 20px; background:#b30000; color:white; text-decoration:none; border-radius:6px; font-weight:bold;">← BACK TO DASHBOARD</a></div>`)
	fmt.Fprintln(w, "<h1>Payment Reports</h1>")

	for rows.Next() {

		var (
			id           int64
			orderID      int64
			customerID   int64
			customerName string
			phone        string
			amount       float64
			status       string
			note         sql.NullString
			createdAt    string
		)

		err := rows.Scan(
			&id,
			&orderID,
			&customerID,
			&customerName,
			&phone,
			&amount,
			&status,
			&note,
			&createdAt,
		)

		if err != nil {
			http.Error(
				w,
				"Could not read payment report: "+err.Error(),
				http.StatusInternalServerError,
			)
			return
		}

		fmt.Fprintln(w, "<hr>")

		fmt.Fprintf(
			w,
			"<h2>🔔 Payment Report #%d</h2>"+
				"<p><strong>Customer:</strong> %s</p>"+
				"<p><strong>Phone:</strong> %s</p>"+
				"<p><strong>Order:</strong> #%d</p>"+
				"<p><strong>Amount:</strong> ₦%.2f</p>"+
				"<p><strong>Status:</strong> %s</p>"+
				"<p><strong>Note:</strong> %s</p>"+
				"<p><strong>Reported:</strong> %s</p>",
			id,
			customerName,
			phone,
			orderID,
			amount,
			status,
			note.String,
			formatNigeriaTime(createdAt),
		)

		if status == "PENDING" {

			fmt.Fprintf(
				w,
				`<form method="POST" action="/admin/confirm-payment" style="margin:20px 0;">
					<input type="hidden" name="report_id" value="%d">
					<button type="submit"
						style="padding:12px 20px; background:#198754; color:white; border:none; border-radius:6px; cursor:pointer; font-weight:bold;">
						✅ CONFIRM PAYMENT
					</button>
				</form>`,
				id,
			)

		} else if status == "CONFIRMED" {

			fmt.Fprintln(
				w,
				`<p style="color:green; font-weight:bold;">✅ PAYMENT CONFIRMED</p>`,
			)
		}
	}

	if err := rows.Err(); err != nil {
		http.Error(
			w,
			"Could not read payment reports: "+err.Error(),
			http.StatusInternalServerError,
		)
		return
	}

	fmt.Fprintln(w, "</body></html>")
}

// =========================
// ADMIN CONFIRM PAYMENT
// =========================

func adminConfirmPaymentHandler(w http.ResponseWriter, r *http.Request) {

	_, loggedIn := getAdminSession(r)

	if !loggedIn {
		http.Redirect(
			w,
			r,
			"/comfort-spoon-control/login",
			http.StatusSeeOther,
		)
		return
	}

	if r.Method != http.MethodPost {
		http.Error(
			w,
			"Method not allowed",
			http.StatusMethodNotAllowed,
		)
		return
	}

	reportID, err := strconv.ParseInt(
		r.FormValue("report_id"),
		10,
		64,
	)

	if err != nil || reportID <= 0 {
		http.Error(
			w,
			"Invalid payment report",
			http.StatusBadRequest,
		)
		return
	}

	adminID, _ := getAdminSession(r)

	// =========================================================
	// START DATABASE TRANSACTION
	// =========================================================

	tx, err := db.Begin()

	if err != nil {
		http.Error(
			w,
			"Could not start payment confirmation: "+err.Error(),
			http.StatusInternalServerError,
		)
		return
	}

	defer tx.Rollback()

	// =========================================================
	// LOAD THE PENDING PAYMENT REPORT
	// =========================================================

	var orderID int64
	var customerID int64
	var amount float64

	err = tx.QueryRow(`
		SELECT
			order_id,
			customer_id,
			amount
		FROM payment_reports
		WHERE id = ?
		  AND status = 'PENDING'
	`, reportID).Scan(
		&orderID,
		&customerID,
		&amount,
	)

	if err == sql.ErrNoRows {
		http.Error(
			w,
			"Payment report was already confirmed or does not exist.",
			http.StatusBadRequest,
		)
		return
	}

	if err != nil {
		http.Error(
			w,
			"Could not load payment report: "+err.Error(),
			http.StatusInternalServerError,
		)
		return
	}

	if amount <= 0 {
		http.Error(
			w,
			"Invalid payment amount in report.",
			http.StatusBadRequest,
		)
		return
	}

	// =========================================================
	// LOAD THE ORIGINAL ORDER
	// =========================================================

	var total float64
	var currentAmountPaid float64

	err = tx.QueryRow(`
		SELECT
			total_amount,
			amount_paid
		FROM orders
		WHERE id = ?
		  AND customer_id = ?
	`, orderID, customerID).Scan(
		&total,
		&currentAmountPaid,
	)

	if err == sql.ErrNoRows {
		http.Error(
			w,
			"Original order for this payment report could not be found.",
			http.StatusBadRequest,
		)
		return
	}

	if err != nil {
		http.Error(
			w,
			"Could not load original order: "+err.Error(),
			http.StatusInternalServerError,
		)
		return
	}

	// =========================================================
	// CALCULATE THE REAL OUTSTANDING BALANCE
	// =========================================================

	previousBalance := total - currentAmountPaid

	if previousBalance < 0 {
		previousBalance = 0
	}

	if amount > previousBalance && previousBalance > 0 {
		http.Error(
			w,
			fmt.Sprintf(
				"Payment report is greater than the outstanding balance of ₦%.2f",
				previousBalance,
			),
			http.StatusBadRequest,
		)
		return
	}

	if previousBalance == 0 {
		_, err = tx.Exec(`
                        UPDATE payment_reports
                        SET
                                status = 'CONFIRMED',
                                confirmed_at = CURRENT_TIMESTAMP,
                                confirmed_by = ?
                        WHERE id = ?
                `,
			adminID,
			reportID,
		)

		if err != nil {
			http.Error(
				w,
				"Could not confirm payment report: "+err.Error(),
				http.StatusInternalServerError,
			)
			return
		}

		if err := tx.Commit(); err != nil {
			http.Error(
				w,
				"Could not complete confirmation: "+err.Error(),
				http.StatusInternalServerError,
			)
			return
		}

		http.Redirect(
			w,
			r,
			"/admin/payment-reports",
			http.StatusSeeOther,
		)
		return
	}

	balanceRemaining := previousBalance - amount

	if balanceRemaining < 0.01 {
		balanceRemaining = 0
	}

	newTotalPaid := currentAmountPaid + amount

	paymentStatus := "PART PAYMENT"

	if balanceRemaining == 0 {
		paymentStatus = "PAID"
	}

	// =========================================================
	// CREATE THE ACTUAL PAYMENT TRANSACTION
	// =========================================================

	_, err = tx.Exec(`
		INSERT INTO payments (
			order_id,
			customer_id,
			payment_type,
			previous_balance,
			amount_paid,
			balance_remaining,
			payment_status,
			report_id
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`,
		orderID,
		customerID,
		"OUTSTANDING BALANCE PAYMENT",
		previousBalance,
		amount,
		balanceRemaining,
		paymentStatus,
		reportID,
	)

	if err != nil {
		http.Error(
			w,
			"Could not record confirmed payment: "+err.Error(),
			http.StatusInternalServerError,
		)
		return
	}

	// =========================================================
	// UPDATE THE ORIGINAL ORDER
	// =========================================================

	_, err = tx.Exec(`
		UPDATE orders
		SET
			amount_paid = ?,
			payment_status = ?,
			last_payment_date = CURRENT_TIMESTAMP
		WHERE id = ?
		  AND customer_id = ?
	`,
		newTotalPaid,
		paymentStatus,
		orderID,
		customerID,
	)

	if err != nil {
		http.Error(
			w,
			"Could not update original order: "+err.Error(),
			http.StatusInternalServerError,
		)
		return
	}

	// =========================================================
	// IF FULLY PAID, CLOSE THE CUSTOMER'S ACTIVE ORDER
	// =========================================================

	if paymentStatus == "PAID" {

		_, err = tx.Exec(`
			UPDATE customers
			SET active_order_id = NULL
			WHERE id = ?
			  AND active_order_id = ?
		`,
			customerID,
			orderID,
		)

		if err != nil {
			http.Error(
				w,
				"Could not close paid order: "+err.Error(),
				http.StatusInternalServerError,
			)
			return
		}
	}

	// =========================================================
	// MARK PAYMENT REPORT AS CONFIRMED
	// =========================================================

	result, err := tx.Exec(`
		UPDATE payment_reports
		SET
			status = 'CONFIRMED',
			confirmed_at = CURRENT_TIMESTAMP,
			confirmed_by = ?
		WHERE id = ?
		  AND status = 'PENDING'
	`,
		adminID,
		reportID,
	)

	if err != nil {
		http.Error(
			w,
			"Could not confirm payment report: "+err.Error(),
			http.StatusInternalServerError,
		)
		return
	}

	rowsAffected, err := result.RowsAffected()

	if err != nil {
		http.Error(
			w,
			"Could not verify payment confirmation: "+err.Error(),
			http.StatusInternalServerError,
		)
		return
	}

	if rowsAffected == 0 {
		http.Error(
			w,
			"Payment report was already confirmed or does not exist.",
			http.StatusBadRequest,
		)
		return
	}

	// =========================================================
	// COMMIT EVERYTHING
	// =========================================================

	if err := tx.Commit(); err != nil {
		http.Error(
			w,
			"Could not complete payment confirmation: "+err.Error(),
			http.StatusInternalServerError,
		)
		return
	}

	log.Println("========== PAYMENT CONFIRMED ==========")
	log.Println("Payment Report ID:", reportID)
	log.Println("Order ID:", orderID)
	log.Println("Customer ID:", customerID)
	log.Println("Previous Balance:", previousBalance)
	log.Println("Payment Confirmed:", amount)
	log.Println("Balance Remaining:", balanceRemaining)
	log.Println("Payment Status:", paymentStatus)
	log.Println("Confirmed By Admin ID:", adminID)
	log.Println("=======================================")

	http.Redirect(
		w,
		r,
		"/admin/payment-reports",
		http.StatusSeeOther,
	)
}

// =========================
// ADMIN
// =========================

func adminHandler(w http.ResponseWriter, r *http.Request) {

	_, loggedIn := getAdminSession(r)

	if !loggedIn {
		http.Redirect(
			w,
			r,
			"/comfort-spoon-control/login",
			http.StatusSeeOther,
		)
		return
	}

	// =========================================================
	// LOAD ALL FABRICS
	// =========================================================

	rows, err := db.Query(`
		SELECT id, name, description, price, quantity, image
		FROM products
		ORDER BY id DESC
	`)

	if err != nil {
		http.Error(
			w,
			"Could not load fabrics: "+err.Error(),
			http.StatusInternalServerError,
		)
		return
	}

	defer rows.Close()

	var products []Product

	for rows.Next() {

		var product Product

		err := rows.Scan(
			&product.ID,
			&product.Name,
			&product.Description,
			&product.Price,
			&product.Quantity,
			&product.Image,
		)

		if err != nil {
			http.Error(
				w,
				"Could not read fabrics: "+err.Error(),
				http.StatusInternalServerError,
			)
			return
		}

		products = append(products, product)
	}

	if err := rows.Err(); err != nil {
		http.Error(
			w,
			"Could not read fabrics: "+err.Error(),
			http.StatusInternalServerError,
		)
		return
	}

	// =========================================================
	// COUNT PENDING PAYMENT REPORTS
	// =========================================================

	var pendingPayments int

	err = db.QueryRow(`
		SELECT COUNT(*)
		FROM payment_reports
		WHERE status = 'PENDING'
	`).Scan(&pendingPayments)

	if err != nil {
		http.Error(
			w,
			"Could not check pending payments: "+err.Error(),
			http.StatusInternalServerError,
		)
		return
	}
	// =========================================================
	// COUNT PENDING FABRIC REQUESTS
	// =========================================================

	var pendingFabricRequests int

	err = db.QueryRow(`
                SELECT COUNT(*)
                FROM fabric_requests
                WHERE status = 'PENDING'
        `).Scan(&pendingFabricRequests)

	if err != nil {
		http.Error(w, "Could not check pending fabric requests: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// ADMIN DASHBOARD DATA
	// =========================================================

	data := struct {
		Products              []Product
		PendingPayments       int
		PendingFabricRequests int
	}{
		Products:              products,
		PendingPayments:       pendingPayments,
		PendingFabricRequests: pendingFabricRequests,
	}

	// =========================================================
	// LOAD ADMIN TEMPLATE
	// =========================================================

	tmpl, err := template.ParseFiles(
		"templates/admin.html",
	)

	if err != nil {
		http.Error(
			w,
			"Template error: "+err.Error(),
			http.StatusInternalServerError,
		)
		return
	}

	err = tmpl.Execute(w, data)

	if err != nil {
		http.Error(
			w,
			"Page error: "+err.Error(),
			http.StatusInternalServerError,
		)
		return
	}
}

// =========================
// ADMIN SESSION HELPERS
// =========================

func setAdminSession(w http.ResponseWriter, adminID int) {

	http.SetCookie(w, &http.Cookie{
		Name:     "comfort_spoon_admin",
		Value:    strconv.Itoa(adminID),
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   60 * 60,
	})
}

func getAdminSession(r *http.Request) (int, bool) {

	cookie, err := r.Cookie("comfort_spoon_admin")

	if err != nil {
		return 0, false
	}

	adminID, err := strconv.Atoi(cookie.Value)

	if err != nil || adminID <= 0 {
		return 0, false
	}

	var verifiedAdminID int

	err = db.QueryRow(`
                SELECT id
                FROM admins
                WHERE id = ?
        `, adminID).Scan(&verifiedAdminID)

	if err != nil {
		return 0, false
	}

	return verifiedAdminID, true
}

// =========================
// ADMIN LOGIN
// =========================

func adminLoginHandler(w http.ResponseWriter, r *http.Request) {

	if r.Method == http.MethodGet {
		message := r.URL.Query().Get("message")

		renderTemplate(
			w,
			"templates/admin_login.html",
			struct {
				Message string
			}{
				Message: message,
			},
		)
		return
	}

	if r.Method != http.MethodPost {
		http.Error(
			w,
			"Method not allowed",
			http.StatusMethodNotAllowed,
		)
		return
	}

	err := r.ParseForm()

	if err != nil {
		http.Error(
			w,
			"Could not process login",
			http.StatusBadRequest,
		)
		return
	}

	username := r.FormValue("username")
	password := r.FormValue("password")

	if username == "" || password == "" {
		http.Redirect(
			w,
			r,
			"/comfort-spoon-control/login?message=Please+enter+your+username+and+password.",
			http.StatusSeeOther,
		)
		return
	}

	var adminID int

	err = db.QueryRow(`
                SELECT id
                FROM admins
                WHERE username = ?
                  AND password = ?
        `,
		username,
		password,
	).Scan(&adminID)

	if err == sql.ErrNoRows {
		http.Redirect(
			w,
			r,
			"/comfort-spoon-control/login?message=The+username+or+password+is+incorrect.+Please+try+again.",
			http.StatusSeeOther,
		)
		return
	}

	if err != nil {
		http.Error(
			w,
			"Could not login: "+err.Error(),
			http.StatusInternalServerError,
		)
		return
	}

	setAdminSession(w, adminID)

	http.Redirect(
		w,
		r,
		"/admin",
		http.StatusSeeOther,
	)
}

// =========================
// ADMIN LOGOUT
// =========================

func adminLogoutHandler(w http.ResponseWriter, r *http.Request) {

	if r.Method != http.MethodGet {
		http.Error(
			w,
			"Method not allowed",
			http.StatusMethodNotAllowed,
		)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "comfort_spoon_admin",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	http.Redirect(
		w,
		r,
		"/",
		http.StatusSeeOther,
	)
}

// =========================
// ADD FABRIC
// =========================

func addFabricHandler(w http.ResponseWriter, r *http.Request) {

	_, loggedIn := getAdminSession(r)

	if !loggedIn {
		http.Redirect(
			w,
			r,
			"/comfort-spoon-control/login",
			http.StatusSeeOther,
		)
		return
	}

	if r.Method == http.MethodGet {
		renderTemplate(w, "templates/add_fabric.html", nil)
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	err := r.ParseMultipartForm(10 << 20)
	if err != nil {
		http.Error(w, "Could not process form", http.StatusBadRequest)
		return
	}

	name := r.FormValue("name")
	description := r.FormValue("description")

	if name == "" {
		http.Error(w, "Fabric name is required", http.StatusBadRequest)
		return
	}

	// Check whether this fabric name already exists.
	var existingID int
	var existingName string
	var existingDescription string
	var existingPrice float64
	var existingQuantity int
	var existingImage string

	err = db.QueryRow(`
                SELECT id, name, description, price, quantity, image
                FROM products
                WHERE LOWER(name) = LOWER(?)
                LIMIT 1
        `,
		name,
	).Scan(
		&existingID,
		&existingName,
		&existingDescription,
		&existingPrice,
		&existingQuantity,
		&existingImage,
	)

	if err == nil {
		type DuplicateFabricPage struct {
			ID          int
			Name        string
			Description string
			Price       float64
			Quantity    string
			Image       string
		}

		data := DuplicateFabricPage{
			ID:          existingID,
			Name:        existingName,
			Description: existingDescription,
			Price:       existingPrice,
			Quantity:    fmt.Sprintf("%d", existingQuantity),
			Image:       existingImage,
		}

		renderTemplate(
			w,
			"templates/duplicate_fabric.html",
			data,
		)
		return
	}

	if err != sql.ErrNoRows {
		http.Error(
			w,
			"Could not check existing fabric: "+err.Error(),
			http.StatusInternalServerError,
		)
		return
	}

	price, err := strconv.ParseFloat(r.FormValue("price"), 64)
	if err != nil || price < 0 {
		http.Error(w, "Invalid price", http.StatusBadRequest)
		return
	}

	quantity, err := strconv.Atoi(r.FormValue("quantity"))
	if err != nil || quantity < 0 {
		http.Error(w, "Invalid quantity", http.StatusBadRequest)
		return
	}

	imagePath := ""

	file, header, err := r.FormFile("image")

	if err == nil {
		defer file.Close()

		err = os.MkdirAll(getUploadDir(), 0755)

		if err != nil {
			http.Error(w, "Could not create upload folder", http.StatusInternalServerError)
			return
		}

		err = os.MkdirAll(getUploadDir(), 0755)
		if err != nil {
			http.Error(w, "Could not create upload folder", http.StatusInternalServerError)
			return
		}

		extension := filepath.Ext(header.Filename)

		filename := strconv.FormatInt(time.Now().UnixNano(), 10) + extension

		savePath := filepath.Join(getUploadDir(), filename)

		destination, err := os.Create(savePath)
		if err != nil {
			http.Error(w, "Could not save picture", http.StatusInternalServerError)
			return
		}
		defer destination.Close()

		_, err = destination.ReadFrom(file)
		if err != nil {
			http.Error(w, "Could not save picture", http.StatusInternalServerError)
			return
		}

		imagePath = "/uploads/fabrics/" + filename
	}

	_, err = db.Exec(`
		INSERT INTO products
		(name, description, price, quantity, image)
		VALUES (?, ?, ?, ?, ?)
	`,
		name,
		description,
		price,
		quantity,
		imagePath,
	)

	if err != nil {
		http.Error(w, "Could not save fabric: "+err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}

// =========================
// DELETE FABRIC
// =========================

func deleteFabricHandler(w http.ResponseWriter, r *http.Request) {

	_, loggedIn := getAdminSession(r)

	if !loggedIn {
		http.Redirect(
			w,
			r,
			"/comfort-spoon-control/login",
			http.StatusSeeOther,
		)
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id := r.FormValue("id")

	if id == "" {
		http.Error(w, "Fabric ID is required", http.StatusBadRequest)
		return
	}

	_, err := db.Exec(
		"DELETE FROM products WHERE id = ?",
		id,
	)

	if err != nil {
		http.Error(w, "Could not delete fabric: "+err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}

// =========================
// MAIN
// =========================

// =========================
// EDIT FABRIC
// =========================

func editFabricHandler(w http.ResponseWriter, r *http.Request) {

	_, loggedIn := getAdminSession(r)

	if !loggedIn {
		http.Redirect(
			w,
			r,
			"/comfort-spoon-control/login",
			http.StatusSeeOther,
		)
		return
	}

	if r.Method == http.MethodGet {

		id := r.URL.Query().Get("id")

		if id == "" {
			http.Error(
				w,
				"Fabric ID is missing",
				http.StatusBadRequest,
			)
			return
		}

		var product Product

		err := db.QueryRow(`
			SELECT id, name, description, price, quantity, image
			FROM products
			WHERE id = ?
		`, id).Scan(
			&product.ID,
			&product.Name,
			&product.Description,
			&product.Price,
			&product.Quantity,
			&product.Image,
		)

		if err != nil {
			http.Error(
				w,
				"Fabric not found: "+err.Error(),
				http.StatusNotFound,
			)
			return
		}

		tmpl, err := template.ParseFiles(
			"templates/edit_fabric.html",
		)

		if err != nil {
			http.Error(
				w,
				"Template error: "+err.Error(),
				http.StatusInternalServerError,
			)
			return
		}

		err = tmpl.Execute(w, product)

		if err != nil {
			http.Error(
				w,
				"Could not display edit page: "+err.Error(),
				http.StatusInternalServerError,
			)
		}

		return
	}

	if r.Method != http.MethodPost {

		http.Error(
			w,
			"Method not allowed",
			http.StatusMethodNotAllowed,
		)

		return
	}

	err := r.ParseMultipartForm(10 << 20)

	if err != nil {

		http.Error(
			w,
			"Could not process form",
			http.StatusBadRequest,
		)

		return
	}

	id := r.FormValue("id")
	name := r.FormValue("name")
	description := r.FormValue("description")

	price, err := strconv.ParseFloat(
		r.FormValue("price"),
		64,
	)

	if err != nil {

		http.Error(
			w,
			"Invalid price",
			http.StatusBadRequest,
		)

		return
	}

	quantity, err := strconv.Atoi(
		r.FormValue("quantity"),
	)

	if err != nil {

		http.Error(
			w,
			"Invalid quantity",
			http.StatusBadRequest,
		)

		return
	}

	// Keep the existing image
	var oldImage string

	err = db.QueryRow(
		"SELECT image FROM products WHERE id = ?",
		id,
	).Scan(&oldImage)

	if err != nil {

		http.Error(
			w,
			"Fabric not found",
			http.StatusNotFound,
		)

		return
	}

	imagePath := oldImage

	// Check if a new image was uploaded
	file, header, err := r.FormFile("image")

	if err == nil {

		defer file.Close()

		err = os.MkdirAll(getUploadDir(), 0755)

		if err != nil {
			http.Error(w, "Could not create upload folder", http.StatusInternalServerError)
			return
		}

		err = os.MkdirAll(
			getUploadDir(),
			0755,
		)

		if err != nil {

			http.Error(
				w,
				"Could not create upload folder",
				http.StatusInternalServerError,
			)

			return
		}

		extension := filepath.Ext(
			header.Filename,
		)

		filename := strconv.FormatInt(
			time.Now().UnixNano(),
			10,
		) + extension

		savePath := filepath.Join(
			getUploadDir(),
			filename,
		)

		destination, err := os.Create(savePath)

		if err != nil {

			http.Error(
				w,
				"Could not save new picture",
				http.StatusInternalServerError,
			)

			return
		}

		defer destination.Close()

		_, err = destination.ReadFrom(file)

		if err != nil {

			http.Error(
				w,
				"Could not save new picture",
				http.StatusInternalServerError,
			)

			return
		}

		imagePath = "/uploads/fabrics/" + filename
	}

	// Update fabric
	_, err = db.Exec(`
		UPDATE products
		SET name = ?,
		    description = ?,
		    price = ?,
		    quantity = ?,
		    image = ?
		WHERE id = ?
	`,
		name,
		description,
		price,
		quantity,
		imagePath,
		id,
	)

	if err != nil {

		http.Error(
			w,
			"Could not update fabric: "+err.Error(),
			http.StatusInternalServerError,
		)

		return
	}

	// Back to admin
	http.Redirect(
		w,
		r,
		"/admin",
		http.StatusSeeOther,
	)
}
func orderHandler(w http.ResponseWriter, r *http.Request) {

	if r.Method == http.MethodGet {

		customerID, loggedIn := getCustomerSession(r)

		if !loggedIn || customerID <= 0 {
			http.Redirect(
				w,
				r,
				"/register?message=Please+register+or+log+in+before+placing+an+order.",
				http.StatusSeeOther,
			)
			return
		}

		var customer struct {
			Name  string
			Phone string
		}

		err := db.QueryRow(`
                SELECT full_name, phone
                FROM customers
                WHERE id = ?
        `, customerID).Scan(
			&customer.Name,
			&customer.Phone,
		)

		if err != nil {
			http.Error(
				w,
				"Could not load customer details: "+err.Error(),
				http.StatusInternalServerError,
			)
			return
		}

		data := struct {
			Name  string
			Phone string
		}{
			Name:  customer.Name,
			Phone: customer.Phone,
		}

		renderTemplate(
			w,
			"templates/order.html",
			data,
		)
		return
	}

	if r.Method != http.MethodPost {
		http.Error(
			w,
			"Method not allowed",
			http.StatusMethodNotAllowed,
		)
		return
	}

	err := r.ParseForm()
	if err != nil {
		http.Error(
			w,
			"Could not process order",
			http.StatusBadRequest,
		)
		return
	}

	name := r.FormValue("name")
	phone := r.FormValue("phone")
	address := r.FormValue("address")
	note := r.FormValue("note")
	cartJSON := r.FormValue("cart")

	log.Printf("ORDER CART RECEIVED: %s", cartJSON)

	if name == "" || phone == "" {
		http.Error(
			w,
			"Name and phone number are required",
			http.StatusBadRequest,
		)
		return
	}

	if cartJSON == "" {
		http.Error(
			w,
			"Your cart is empty",
			http.StatusBadRequest,
		)
		return
	}

	var cart []struct {
		ID       int     `json:"id"`
		Name     string  `json:"name"`
		Price    float64 `json:"price"`
		Quantity int     `json:"quantity"`
	}

	err = json.Unmarshal([]byte(cartJSON), &cart)
	if err != nil {
		http.Error(
			w,
			"Could not read your cart",
			http.StatusBadRequest,
		)
		return
	}

	if len(cart) == 0 {
		http.Error(
			w,
			"Your cart is empty",
			http.StatusBadRequest,
		)
		return
	}

	// ---------------------------------------------------------
	// FIND REGISTERED CUSTOMER
	// ---------------------------------------------------------

	var customerID int

	err = db.QueryRow(`
                SELECT id
                FROM customers
                WHERE phone = ?
        `, phone).Scan(&customerID)

	if err == sql.ErrNoRows {
		http.Error(
			w,
			"Please register before placing an order.",
			http.StatusBadRequest,
		)
		return
	}

	if err != nil {
		http.Error(
			w,
			"Could not find customer: "+err.Error(),
			http.StatusInternalServerError,
		)
		return
	}

	// ---------------------------------------------------------
	// CALCULATE NEW FABRICS TOTAL
	// ---------------------------------------------------------

	var total float64

	for _, item := range cart {

		if item.Quantity <= 0 || item.Price < 0 {
			http.Error(
				w,
				"Invalid item in cart",
				http.StatusBadRequest,
			)
			return
		}

		total += item.Price * float64(item.Quantity)
	}

	if total <= 0 {
		http.Error(
			w,
			"Order total must be greater than zero",
			http.StatusBadRequest,
		)
		return
	}

	// ---------------------------------------------------------
	// FIND OUTSTANDING BALANCE
	// ---------------------------------------------------------

	var previousBalance float64
	var previousBalanceOrderID int64

	err = db.QueryRow(`
                SELECT
                        total_amount - amount_paid,
                        id
                FROM orders
                WHERE customer_id = ?
                  AND amount_paid < total_amount
                  AND carried_forward_to_order_id IS NULL
                ORDER BY id DESC
                LIMIT 1
        `,
		customerID,
	).Scan(
		&previousBalance,
		&previousBalanceOrderID,
	)

	if err == sql.ErrNoRows {
		previousBalance = 0
		previousBalanceOrderID = 0
	} else if err != nil {
		http.Error(
			w,
			"Could not check previous balance: "+err.Error(),
			http.StatusInternalServerError,
		)
		return
	}

	totalWithBalance := total + previousBalance

	// ---------------------------------------------------------
	// IMPORTANT:
	//
	// NO ORDER IS CREATED HERE.
	// NO ORDER ITEMS ARE CREATED HERE.
	// NO PAYMENT IS CREATED HERE.
	//
	// We only carry the customer's order information forward
	// to the payment page.
	// ---------------------------------------------------------

	type PendingOrder struct {
		CustomerID             int
		Name                   string
		Phone                  string
		Address                string
		Note                   string
		Cart                   string
		NewFabricTotal         float64
		PreviousBalance        float64
		PreviousBalanceOrderID int64
		Total                  float64
	}

	pending := PendingOrder{
		CustomerID:             customerID,
		Name:                   name,
		Phone:                  phone,
		Address:                address,
		Note:                   note,
		Cart:                   cartJSON,
		NewFabricTotal:         total,
		PreviousBalance:        previousBalance,
		PreviousBalanceOrderID: previousBalanceOrderID,
		Total:                  totalWithBalance,
	}

	// Encode the pending order into the payment URL.
	//
	// Nothing is written to the database here.
	query := url.Values{}
	query.Set("customer", strconv.Itoa(pending.CustomerID))
	query.Set("name", pending.Name)
	query.Set("phone", pending.Phone)
	query.Set("address", pending.Address)
	query.Set("note", pending.Note)
	query.Set("cart", pending.Cart)

	http.Redirect(
		w,
		r,
		"/payment?"+query.Encode(),
		http.StatusSeeOther,
	)
}

func receiptHandler(w http.ResponseWriter, r *http.Request) {

	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	orderID := r.URL.Query().Get("order")

	if orderID == "" {
		http.Error(w, "Order number is required", http.StatusBadRequest)
		return
	}

	type ReceiptItem struct {
		ProductName string
		Price       float64
		Quantity    int
		Subtotal    float64
	}

	type Receipt struct {
		ReceiptTitle            string
		IsOutstandingPayment    bool
		PaymentID               int64
		PaymentPreviousBalance  float64
		PaymentAmount           float64
		PaymentBalanceRemaining float64
		PaymentDate             string
		OrderID                 int64
		Date                    string
		CustomerName            string
		Phone                   string
		Items                   []ReceiptItem
		Total                   float64
		AmountPaid              float64
		Balance                 float64
		PaymentStatus           string
		PreviousBalance         float64
		PreviousBalanceOrderID  int64
		LastPayment             string
	}

	var receipt Receipt

	receipt.ReceiptTitle = "ORDER RECEIPT"

	if r.URL.Query().Get("payment") == "outstanding" {
		receipt.ReceiptTitle = "OUTSTANDING PAYMENT RECEIPT"
		receipt.IsOutstandingPayment = true
	}

	err := db.QueryRow(`
		SELECT
			o.id,
			o.created_at,
			c.full_name,
			c.phone,
			o.total_amount,
			o.amount_paid,
			o.payment_status,
                        o.previous_balance,
                        COALESCE(o.previous_balance_order_id, 0),
			COALESCE(o.last_payment_date, '')
		FROM orders o
		JOIN customers c ON c.id = o.customer_id
		WHERE o.id = ?
	`, orderID).Scan(
		&receipt.OrderID,
		&receipt.Date,
		&receipt.CustomerName,
		&receipt.Phone,
		&receipt.Total,
		&receipt.AmountPaid,
		&receipt.PaymentStatus,
		&receipt.PreviousBalance,
		&receipt.PreviousBalanceOrderID,
		&receipt.LastPayment,
	)

	if err == sql.ErrNoRows {
		http.Error(w, "Order not found", http.StatusNotFound)
		return
	}

	if err != nil {
		http.Error(
			w,
			"Could not load receipt: "+err.Error(),
			http.StatusInternalServerError,
		)
		return
	}

	receipt.Date = formatNigeriaDate(receipt.Date)
	receipt.LastPayment = formatNigeriaDate(receipt.LastPayment)

	receipt.Balance = receipt.Total - receipt.AmountPaid

	// OUTSTANDING PAYMENT RECEIPT
	if receipt.IsOutstandingPayment {

		err = db.QueryRow(`
				SELECT
						id,
						previous_balance,
						amount_paid,
						balance_remaining,
						created_at
				FROM payments
				WHERE order_id = ?
				ORDER BY id DESC
				LIMIT 1
			`, orderID).Scan(
			&receipt.PaymentID,
			&receipt.PaymentPreviousBalance,
			&receipt.PaymentAmount,
			&receipt.PaymentBalanceRemaining,
			&receipt.PaymentDate,
		)

		if err == sql.ErrNoRows {
			http.Error(
				w,
				"Payment transaction not found",
				http.StatusNotFound,
			)
			return
		}

		if err != nil {
			http.Error(
				w,
				"Could not load payment receipt: "+err.Error(),
				http.StatusInternalServerError,
			)
			return
		}

		receipt.PaymentDate = formatNigeriaDate(receipt.PaymentDate)
	}

	// OUTSTANDING PAYMENT RECEIPT
	if receipt.IsOutstandingPayment {

		err = db.QueryRow(`
				SELECT
						id,
						previous_balance,
						amount_paid,
						balance_remaining,
						created_at
				FROM payments
				WHERE order_id = ?
				ORDER BY id DESC
				LIMIT 1
			`, orderID).Scan(
			&receipt.PaymentID,
			&receipt.PaymentPreviousBalance,
			&receipt.PaymentAmount,
			&receipt.PaymentBalanceRemaining,
			&receipt.PaymentDate,
		)

		if err == sql.ErrNoRows {
			http.Error(
				w,
				"Payment transaction not found",
				http.StatusNotFound,
			)
			return
		}

		if err != nil {
			http.Error(
				w,
				"Could not load payment receipt: "+err.Error(),
				http.StatusInternalServerError,
			)
			return
		}

		receipt.PaymentDate = formatNigeriaDate(receipt.PaymentDate)
	}

	err = db.QueryRow(`
		SELECT COUNT(*)
		FROM order_items
		WHERE order_id = ?
	`, orderID).Scan(new(int))

	if err != nil {
		http.Error(
			w,
			"Could not check order items: "+err.Error(),
			http.StatusInternalServerError,
		)
		return
	}

	rows, err := db.Query(`
		SELECT product_name, price, quantity, subtotal
		FROM order_items
		WHERE order_id = ?
		ORDER BY id DESC
	`, orderID)

	if err != nil {
		http.Error(
			w,
			"Could not load order items: "+err.Error(),
			http.StatusInternalServerError,
		)
		return
	}

	defer rows.Close()

	for rows.Next() {

		var item ReceiptItem

		err := rows.Scan(
			&item.ProductName,
			&item.Price,
			&item.Quantity,
			&item.Subtotal,
		)

		if err != nil {
			http.Error(
				w,
				"Could not read order items: "+err.Error(),
				http.StatusInternalServerError,
			)
			return
		}

		receipt.Items = append(receipt.Items, item)
	}

	if err := rows.Err(); err != nil {
		http.Error(
			w,
			"Could not read order items: "+err.Error(),
			http.StatusInternalServerError,
		)
		return
	}

	renderTemplate(
		w,
		"templates/receipt.html",
		receipt,
	)
}

// =========================
// CUSTOMER REGISTRATION
// =========================

// =========================
// CUSTOMER REGISTRATION
// =========================

func registerHandler(w http.ResponseWriter, r *http.Request) {

	if r.Method == http.MethodGet {
		message := r.URL.Query().Get("message")

		renderTemplate(
			w,
			"templates/register.html",
			struct {
				Message string
			}{
				Message: message,
			},
		)
		return
	}

	if r.Method != http.MethodPost {
		http.Error(
			w,
			"Method not allowed",
			http.StatusMethodNotAllowed,
		)
		return
	}

	err := r.ParseForm()

	if err != nil {
		http.Error(
			w,
			"Could not process registration",
			http.StatusBadRequest,
		)
		return
	}

	name := r.FormValue("name")
	phone := r.FormValue("phone")

	if name == "" || phone == "" {

		http.Error(
			w,
			"Full name and phone number are required",
			http.StatusBadRequest,
		)

		return
	}

	if !validPhone(phone) {

		renderTemplate(
			w,
			"templates/error.html",
			"Phone number must contain exactly 11 digits. Please enter a valid 11-digit phone number.",
		)

		return
	}

	// Check if phone number already exists
	var existingID int

	err = db.QueryRow(
		"SELECT id FROM customers WHERE phone = ?",
		phone,
	).Scan(&existingID)

	if err == nil {

		http.Redirect(
			w,
			r,
			"/login?message="+url.QueryEscape(
				"You already have an The Comfort Spoon account with this phone number. Please log in to continue.",
			),
			http.StatusSeeOther,
		)

		return
	}

	// Create customer
	_, err = db.Exec(`
        INSERT INTO customers
        (full_name, phone)
        VALUES (?, ?)
    `,
		name,
		phone,
	)

	if err != nil {

		http.Error(
			w,
			"Could not create account: "+err.Error(),
			http.StatusInternalServerError,
		)

		return
	}

	// Registration successful
	http.Redirect(
		w,
		r,
		"/login",
		http.StatusSeeOther,
	)
}

// =========================
// CUSTOMER LOGIN
// =========================

// =========================
// CUSTOMER LOGIN
// =========================

func loginHandler(w http.ResponseWriter, r *http.Request) {

	if r.Method == http.MethodGet {

		message := r.URL.Query().Get("message")

		renderTemplate(
			w,
			"templates/login.html",
			struct {
				Message string
			}{
				Message: message,
			},
		)

		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	err := r.ParseForm()
	if err != nil {
		http.Error(w, "Could not process login", http.StatusBadRequest)
		return
	}

	phone := r.FormValue("phone")

	if phone == "" {
		http.Error(w, "Phone number is required", http.StatusBadRequest)
		return
	}

	var customerID int
	var customerName string

	err = db.QueryRow(`
		SELECT id, full_name
		FROM customers
		WHERE phone = ?
	`, phone).Scan(
		&customerID,
		&customerName,
	)

	if err == sql.ErrNoRows {
		http.Redirect(
			w,
			r,
			"/register?message="+url.QueryEscape(
				"We couldn't find an account with that phone number. No worries — you can create your account here.",
			),
			http.StatusSeeOther,
		)
		return
	}

	if err != nil {
		http.Error(
			w,
			"Could not login: "+err.Error(),
			http.StatusInternalServerError,
		)
		return
	}

	log.Println("Customer logged in:", customerName)
	setCustomerSession(w, customerID)

	http.Redirect(
		w,
		r,
		"/customer?phone="+url.QueryEscape(phone)+"&new_login=1",
		http.StatusSeeOther,
	)
}

// =========================
// PAY OUTSTANDING BALANCE
// =========================

func outstandingPaymentHandler(w http.ResponseWriter, r *http.Request) {

	customerID, ok := getCustomerSession(r)

	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	// =========================================================
	// GET: SHOW CURRENT OUTSTANDING BALANCE
	// =========================================================

	if r.Method == http.MethodGet {

		var orderID int64
		var total float64
		var amountPaid float64
		var previousBalance float64

		err := db.QueryRow(`
			SELECT
				id,
				total_amount,
				amount_paid,
				COALESCE(previous_balance, 0)
			FROM orders
			WHERE customer_id = ?
			  AND amount_paid < total_amount
			ORDER BY id DESC
			LIMIT 1
		`, customerID).Scan(
			&orderID,
			&total,
			&amountPaid,
			&previousBalance,
		)

		if err == sql.ErrNoRows {

			type OutstandingPage struct {
				HasBalance bool
				Balance    float64
			}

			renderTemplate(
				w,
				"templates/outstanding.html",
				OutstandingPage{
					HasBalance: false,
					Balance:    0,
				},
			)

			return
		}

		if err != nil {
			http.Error(
				w,
				"Could not load outstanding balance: "+err.Error(),
				http.StatusInternalServerError,
			)
			return
		}

		balance := total - amountPaid

		if balance < 0 {
			balance = 0
		}

		type OutstandingPage struct {
			HasBalance      bool
			OrderID         int64
			Total           float64
			AmountPaid      float64
			Balance         float64
			PreviousBalance float64
		}

		renderTemplate(
			w,
			"templates/outstanding.html",
			OutstandingPage{
				HasBalance:      balance > 0,
				OrderID:         orderID,
				Total:           total,
				AmountPaid:      amountPaid,
				Balance:         balance,
				PreviousBalance: previousBalance,
			},
		)

		return
	}

	// =========================================================
	// POST: PAY EXISTING OUTSTANDING BALANCE
	// =========================================================

	if r.Method != http.MethodPost {
		http.Error(
			w,
			"Method not allowed",
			http.StatusMethodNotAllowed,
		)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(
			w,
			"Could not process payment",
			http.StatusBadRequest,
		)
		return
	}

	amountText := r.FormValue("amount_paid")

	amount, err := strconv.ParseFloat(amountText, 64)

	if err != nil || amount <= 0 {
		http.Error(
			w,
			"Please enter a valid payment amount",
			http.StatusBadRequest,
		)
		return
	}

	// ---------------------------------------------------------
	// FIND THE EXISTING ORDER THAT OWNS THE DEBT.
	// NO NEW ORDER IS CREATED HERE.
	// ---------------------------------------------------------

	var orderID int64
	var total float64
	var currentAmountPaid float64

	err = db.QueryRow(`
		SELECT
			id,
			total_amount,
			amount_paid
		FROM orders
		WHERE customer_id = ?
		  AND amount_paid < total_amount
		ORDER BY id DESC
		LIMIT 1
	`, customerID).Scan(
		&orderID,
		&total,
		&currentAmountPaid,
	)

	if err == sql.ErrNoRows {
		http.Error(
			w,
			"You currently have no outstanding balance.",
			http.StatusBadRequest,
		)
		return
	}

	if err != nil {
		http.Error(
			w,
			"Could not find outstanding order: "+err.Error(),
			http.StatusInternalServerError,
		)
		return
	}

	// ---------------------------------------------------------
	// CALCULATE THE DEBT BEFORE AND AFTER THIS PAYMENT.
	// ---------------------------------------------------------

	previousBalance := total - currentAmountPaid

	if previousBalance < 0 {
		previousBalance = 0
	}

	if amount > previousBalance {
		http.Error(
			w,
			fmt.Sprintf(
				"Payment is greater than your outstanding balance of ₦%.2f",
				previousBalance,
			),
			http.StatusBadRequest,
		)
		return
	}

	balanceRemaining := previousBalance - amount

	if balanceRemaining < 0 {
		balanceRemaining = 0
	}

	_, err = db.Exec(`
		INSERT INTO payment_reports
		(
			order_id,
			customer_id,
			amount,
			status,
			customer_note
		)
		VALUES (?, ?, ?, 'PENDING', ?)
	`,
		orderID,
		customerID,
		amount,
		"Customer reported payment of outstanding balance.",
	)

	if err != nil {
		http.Error(
			w,
			"Could not create payment notification: "+err.Error(),
			http.StatusInternalServerError,
		)
		return
	}

	// ---------------------------------------------------------
	// SEND THE CUSTOMER TO THE PAYMENT RECEIPT.
	//
	// This is NOT a new order receipt.
	// ---------------------------------------------------------

	http.Redirect(
		w,
		r,
		"/receipt?order="+strconv.FormatInt(orderID, 10)+"&payment=outstanding",
		http.StatusSeeOther,
	)
}

// =========================
// CUSTOMER FABRIC REQUEST
// =========================

func fabricRequestHandler(w http.ResponseWriter, r *http.Request) {

	customerID, ok := getCustomerSession(r)

	if !ok {
		http.Redirect(
			w,
			r,
			"/login",
			http.StatusSeeOther,
		)
		return
	}

	if r.Method == http.MethodGet {

		renderTemplate(
			w,
			"templates/request_fabric.html",
			nil,
		)

		return
	}

	if r.Method != http.MethodPost {
		http.Error(
			w,
			"Method not allowed",
			http.StatusMethodNotAllowed,
		)
		return
	}

	err := r.ParseMultipartForm(10 << 20)

	if err != nil {
		http.Error(
			w,
			"Could not read form: "+err.Error(),
			http.StatusBadRequest,
		)
		return
	}

	description := r.FormValue("description")
	quantity := r.FormValue("quantity")

	file, header, err := r.FormFile("image")

	imagePath := ""

	if err == nil {

		defer file.Close()

		err = os.MkdirAll(getUploadDir(), 0755)

		if err != nil {
			http.Error(w, "Could not create upload folder", http.StatusInternalServerError)
			return
		}

		extension := filepath.Ext(header.Filename)
		filename := strconv.FormatInt(time.Now().UnixNano(), 10) + extension
		imagePath = "/uploads/fabrics/" + filename

		savePath := filepath.Join(getUploadDir(), filename)

		dst, err := os.Create(savePath)

		if err != nil {
			http.Error(
				w,
				err.Error(),
				http.StatusInternalServerError,
			)
			return
		}

		defer dst.Close()

		io.Copy(dst, file)
	}

	var customerName string
	var phone string

	err = db.QueryRow(`
                SELECT full_name, phone
                FROM customers
                WHERE id = ?
        `, customerID).Scan(&customerName, &phone)

	if err != nil {
		http.Error(
			w,
			"Could not load customer details: "+err.Error(),
			http.StatusInternalServerError,
		)
		return
	}

	_, err = db.Exec(`
                INSERT INTO fabric_requests
                (
                        customer_id,
                        customer_name,
                        phone,
                        image,
                        description,
                        quantity,
                        status
                )
                VALUES (?, ?, ?, ?, ?, ?, 'PENDING')
        `,
		customerID,
		customerName,
		phone,
		imagePath,
		description,
		quantity,
	)

	if err != nil {
		http.Error(
			w,
			"Could not save request: "+err.Error(),
			http.StatusInternalServerError,
		)
		return
	}

	http.Redirect(
		w,
		r,
		"/customer",
		http.StatusSeeOther,
	)
}

// =========================
// CUSTOMER LOGOUT
// =========================

func logoutHandler(w http.ResponseWriter, r *http.Request) {

	if r.Method != http.MethodGet {
		http.Error(
			w,
			"Method not allowed",
			http.StatusMethodNotAllowed,
		)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "comfort_spoon_customer",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	http.Redirect(
		w,
		r,
		"/",
		http.StatusSeeOther,
	)
}

// =========================
// CUSTOMER DASHBOARD
// =========================

func customerHandler(w http.ResponseWriter, r *http.Request) {

	if r.Method != http.MethodGet {
		http.Error(
			w,
			"Method not allowed",
			http.StatusMethodNotAllowed,
		)
		return
	}

	var customer struct {
		ID    int
		Name  string
		Phone string
	}

	customerID, loggedIn := getCustomerSession(r)

	if loggedIn {

		err := db.QueryRow(`
			SELECT id, full_name, phone
			FROM customers
			WHERE id = ?
		`, customerID).Scan(
			&customer.ID,
			&customer.Name,
			&customer.Phone,
		)

		if err != nil {
			http.Redirect(
				w,
				r,
				"/login",
				http.StatusSeeOther,
			)
			return
		}

	} else {

		phone := r.URL.Query().Get("phone")

		if phone == "" {
			http.Redirect(
				w,
				r,
				"/login",
				http.StatusSeeOther,
			)
			return
		}

		err := db.QueryRow(`
			SELECT id, full_name, phone
			FROM customers
			WHERE phone = ?
		`, phone).Scan(
			&customer.ID,
			&customer.Name,
			&customer.Phone,
		)

		if err == sql.ErrNoRows {
			http.Redirect(
				w,
				r,
				"/login",
				http.StatusSeeOther,
			)
			return
		}

		if err != nil {
			http.Error(
				w,
				err.Error(),
				http.StatusInternalServerError,
			)
			return
		}

		setCustomerSession(w, customer.ID)
	}

	renderTemplate(
		w,
		"templates/customer.html",
		customer,
	)
}

// =========================

// =========================
// CUSTOMER FABRIC REQUESTS
// =========================

func myFabricRequestsHandler(w http.ResponseWriter, r *http.Request) {

	customerID, ok := getCustomerSession(r)

	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	type FabricRequest struct {
		ID          int
		Image       string
		Description string
		Quantity    string
		Status      string
		CreatedAt   string
	}

	rows, err := db.Query(`
		SELECT
			id,
			image,
			description,
			quantity,
			status,
			created_at
		FROM fabric_requests
		WHERE customer_id = ?
		ORDER BY id DESC
	`, customerID)

	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	defer rows.Close()

	var requests []FabricRequest

	for rows.Next() {

		var f FabricRequest

		err := rows.Scan(
			&f.ID,
			&f.Image,
			&f.Description,
			&f.Quantity,
			&f.Status,
			&f.CreatedAt,
		)

		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}

		requests = append(requests, f)
	}

	renderTemplate(
		w,
		"templates/my_fabric_requests.html",
		requests,
	)
}

// CUSTOMER FEEDBACK
// =========================

func feedbackHandler(w http.ResponseWriter, r *http.Request) {

	customerID, ok := getCustomerSession(r)

	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	var customerName string

	err := db.QueryRow(`
                SELECT full_name
                FROM customers
                WHERE id = ?
        `, customerID).Scan(&customerName)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if r.Method == http.MethodPost {

		ratingStr := r.FormValue("rating")
		comment := strings.TrimSpace(r.FormValue("comment"))

		rating, err := strconv.Atoi(ratingStr)

		if err != nil || rating < 1 || rating > 5 {
			http.Error(
				w,
				"Please select a rating between 1 and 5 stars.",
				http.StatusBadRequest,
			)
			return
		}

		if comment == "" {
			http.Error(
				w,
				"Please enter your feedback.",
				http.StatusBadRequest,
			)
			return
		}

		_, err = db.Exec(`
                        INSERT INTO feedback
                        (customer_id, customer_name, rating, comment)
                        VALUES (?, ?, ?, ?)
                `,
			customerID,
			customerName,
			rating,
			comment,
		)

		if err != nil {
			http.Error(
				w,
				err.Error(),
				http.StatusInternalServerError,
			)
			return
		}

		http.Redirect(
			w,
			r,
			"/feedback?success=1",
			http.StatusSeeOther,
		)
		return
	}

	rows, err := db.Query(`
                SELECT
                        id,
                        customer_id,
                        customer_name,
                        rating,
                        comment,
                        status,
                        created_at
                FROM feedback
                WHERE status = 'APPROVED'
                ORDER BY id DESC
        `)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	defer rows.Close()

	var feedbackList []Feedback

	for rows.Next() {

		var feedback Feedback

		err := rows.Scan(
			&feedback.ID,
			&feedback.CustomerID,
			&feedback.CustomerName,
			&feedback.Rating,
			&feedback.Comment,
			&feedback.Status,
			&feedback.CreatedAt,
		)

		if err != nil {
			http.Error(
				w,
				err.Error(),
				http.StatusInternalServerError,
			)
			return
		}

		feedbackList = append(feedbackList, feedback)
	}

	type FeedbackPage struct {
		CustomerName string
		Products     []Product
		Feedback     []Feedback
		Success      bool
	}

	page := FeedbackPage{
		CustomerName: customerName,
		Feedback:     feedbackList,
		Success:      r.URL.Query().Get("success") == "1",
	}

	renderTemplate(
		w,
		"templates/feedback.html",
		page,
	)
}

// CUSTOMER ORDER HISTORY
// =========================

func orderHistoryHandler(w http.ResponseWriter, r *http.Request) {

	if r.Method != http.MethodGet {
		http.Error(
			w,
			"Method not allowed",
			http.StatusMethodNotAllowed,
		)
		return
	}

	customerID, ok := getCustomerSession(r)

	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	var phone string

	err := db.QueryRow(`
                SELECT phone
                FROM customers
                WHERE id = ?
        `, customerID).Scan(&phone)

	if err != nil {
		http.Error(
			w,
			"Could not load customer phone: "+err.Error(),
			http.StatusInternalServerError,
		)
		return
	}

	type PaymentHistoryItem struct {
		ID               int64
		PaymentType      string
		PreviousBalance  float64
		AmountPaid       float64
		BalanceRemaining float64
		PaymentStatus    string
		DisplayDate      string
	}

	type OrderHistoryItem struct {
		ID                     int64
		CurrentOrderTotal      float64
		PreviousBalance        float64
		PreviousBalanceOrderID int64
		Total                  float64
		AmountPaid             float64
		Balance                float64
		PaymentStatus          string
		DisplayDate            string
		Payments               []PaymentHistoryItem
	}

	type OrderHistoryPage struct {
		Phone  string
		Orders []OrderHistoryItem
	}

	rows, err := db.Query(`
                SELECT
                        o.id,
                        o.created_at,
                        o.total_amount,
                        o.amount_paid,
                        o.payment_status,
                        COALESCE(o.previous_balance, 0),
                        COALESCE(o.previous_balance_order_id, 0)
                FROM orders o
                WHERE o.customer_id = ?
                ORDER BY o.id DESC
        `, customerID)

	if err != nil {
		http.Error(
			w,
			"Could not load order history: "+err.Error(),
			http.StatusInternalServerError,
		)
		return
	}

	defer rows.Close()

	var orders []OrderHistoryItem

	for rows.Next() {

		var order OrderHistoryItem
		var createdAt string

		err := rows.Scan(
			&order.ID,
			&createdAt,
			&order.Total,
			&order.AmountPaid,
			&order.PaymentStatus,
			&order.PreviousBalance,
			&order.PreviousBalanceOrderID,
		)

		if err != nil {
			http.Error(
				w,
				"Could not read order history: "+err.Error(),
				http.StatusInternalServerError,
			)
			return
		}

		err = db.QueryRow(`
                        SELECT COALESCE(SUM(subtotal), 0)
                        FROM order_items
                        WHERE order_id = ?
                `, order.ID).Scan(&order.CurrentOrderTotal)

		if err != nil {
			http.Error(
				w,
				"Could not calculate order fabrics: "+err.Error(),
				http.StatusInternalServerError,
			)
			return
		}

		order.Balance = order.Total - order.AmountPaid

		if order.Balance < 0 {
			order.Balance = 0
		}

		order.DisplayDate = createdAt

		if parsed, err := time.Parse(time.RFC3339, createdAt); err == nil {
			order.DisplayDate = parsed.In(
				time.FixedZone("WAT", 60*60),
			).Format("02 January 2006, 3:04 PM")
		}

		paymentRows, err := db.Query(`
                        SELECT
                                id,
                                payment_type,
                                COALESCE(previous_balance, 0),
                                amount_paid,
                                COALESCE(balance_remaining, 0),
                                payment_status,
                                created_at
                        FROM payments
                        WHERE order_id = ?
                          AND customer_id = ?
                        ORDER BY id DESC
                `, order.ID, customerID)

		if err != nil {
			http.Error(
				w,
				"Could not load payment history: "+err.Error(),
				http.StatusInternalServerError,
			)
			return
		}

		for paymentRows.Next() {

			var payment PaymentHistoryItem
			var paymentDate string

			err := paymentRows.Scan(
				&payment.ID,
				&payment.PaymentType,
				&payment.PreviousBalance,
				&payment.AmountPaid,
				&payment.BalanceRemaining,
				&payment.PaymentStatus,
				&paymentDate,
			)

			if err != nil {
				paymentRows.Close()

				http.Error(
					w,
					"Could not read payment history: "+err.Error(),
					http.StatusInternalServerError,
				)
				return
			}

			payment.DisplayDate = paymentDate

			if parsed, err := time.Parse(time.RFC3339, paymentDate); err == nil {
				payment.DisplayDate = parsed.In(
					time.FixedZone("WAT", 60*60),
				).Format("02 January 2006, 3:04 PM")
			}

			order.Payments = append(
				order.Payments,
				payment,
			)
		}

		if err := paymentRows.Err(); err != nil {
			paymentRows.Close()

			http.Error(
				w,
				"Could not read payment history: "+err.Error(),
				http.StatusInternalServerError,
			)
			return
		}

		paymentRows.Close()

		orders = append(orders, order)
	}

	if err := rows.Err(); err != nil {
		http.Error(
			w,
			"Could not read order history: "+err.Error(),
			http.StatusInternalServerError,
		)
		return
	}

	data := OrderHistoryPage{
		Phone:  phone,
		Orders: orders,
	}

	renderTemplate(
		w,
		"templates/order-history.html",
		data,
	)
}

// =========================
// CUSTOMER LOGOUT

func checkPaymentTransactionsHandler(w http.ResponseWriter, r *http.Request) {

	_, loggedIn := getAdminSession(r)

	if !loggedIn {
		http.Redirect(
			w,
			r,
			"/comfort-spoon-control/login",
			http.StatusSeeOther,
		)
		return
	}

	rows, err := db.Query(`
		SELECT
			id,
			order_id,
			customer_id,
			payment_type,
			previous_balance,
			amount_paid,
			balance_remaining,
			payment_status,
			created_at
		FROM payments
		ORDER BY id DESC
	`)

	if err != nil {
		http.Error(w, "Could not load payment transactions: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	fmt.Fprintln(w, "<html><body><h1>Payment Transactions</h1>")

	for rows.Next() {

		var id int64
		var orderID int64
		var customerID int64
		var paymentType string
		var previousBalance float64
		var amountPaid float64
		var balanceRemaining float64
		var status string
		var createdAt string

		err := rows.Scan(
			&id,
			&orderID,
			&customerID,
			&paymentType,
			&previousBalance,
			&amountPaid,
			&balanceRemaining,
			&status,
			&createdAt,
		)

		if err != nil {
			http.Error(w, "Could not read payment transaction: "+err.Error(), http.StatusInternalServerError)
			return
		}

		fmt.Fprintf(
			w,
			"<hr><p><strong>Payment ID:</strong> %d</p>"+
				"<p><strong>Order:</strong> #%d</p>"+
				"<p><strong>Customer:</strong> %d</p>"+
				"<p><strong>Payment Type:</strong> %s</p>"+
				"<p><strong>Previous Outstanding:</strong> ₦%.2f</p>"+
				"<p><strong>Amount Paid:</strong> ₦%.2f</p>"+
				"<p><strong>Outstanding Remaining:</strong> ₦%.2f</p>"+
				"<p><strong>Status:</strong> %s</p>"+
				"<p><strong>Date:</strong> %s</p>",
			id,
			orderID,
			customerID,
			paymentType,
			previousBalance,
			amountPaid,
			balanceRemaining,
			status,
			createdAt,
		)
	}

	fmt.Fprintln(w, "</body></html>")
}

func clearTestHistory() {
	_, err := db.Exec(`DELETE FROM payments`)
	if err != nil {
		log.Fatal("Could not clear payments:", err)
	}

	_, err = db.Exec(`DELETE FROM order_items`)
	if err != nil {
		log.Fatal("Could not clear order items:", err)
	}

	_, err = db.Exec(`DELETE FROM orders`)
	if err != nil {
		log.Fatal("Could not clear orders:", err)
	}

	_, err = db.Exec(`UPDATE customers SET active_order_id = NULL`)
	if err != nil {
		log.Fatal("Could not reset active orders:", err)
	}

	log.Println("✅ Order history cleared successfully.")
}

func getPort() string {
	port := os.Getenv("PORT")
	if port == "" {
		return "8080"
	}
	return port
}

func getUploadDir() string {
	dir := os.Getenv("UPLOAD_DIR")
	if dir == "" {
		dir = "uploads/fabrics"
	}
	return dir
}

func getUploadRoot() string {
	return filepath.Dir(getUploadDir())
}

func main() {

	// Connect to database
	initDatabase()
	inspectPaymentSchema()

	// Static files
	http.Handle(
		"/static/",
		http.StripPrefix(
			"/static/",
			http.FileServer(http.Dir("static")),
		),
	)

	// Uploaded fabric images
	http.Handle(
		"/uploads/",
		http.StripPrefix(
			"/uploads/",
			http.FileServer(http.Dir(getUploadRoot())),
		),
	)

	// Website pages
	http.HandleFunc("/", homeHandler)
	http.HandleFunc("/shop", shopHandler)
	http.HandleFunc("/product", productHandler)
	http.HandleFunc("/cart", cartHandler)
	http.HandleFunc("/payment", paymentHandler)
	http.HandleFunc("/flutterwave/pay", flutterwavePayHandler)
	http.HandleFunc("/flutterwave/callback", flutterwaveCallbackHandler)
	http.HandleFunc("/checkout", checkoutHandler)
	http.HandleFunc("/order", orderHandler)
	http.HandleFunc("/receipt", receiptHandler)
	http.HandleFunc("/register", registerHandler)
	http.HandleFunc("/login", loginHandler)
	http.HandleFunc("/logout", logoutHandler)

	http.HandleFunc("/customer", customerHandler)
	http.HandleFunc("/feedback", feedbackHandler)
	http.HandleFunc("/order-history", orderHistoryHandler)
	http.HandleFunc("/fabric-request", fabricRequestHandler)
	http.HandleFunc("/pay-outstanding", outstandingPaymentHandler)
	http.HandleFunc("/my-fabric-requests", myFabricRequestsHandler)

	// Admin
	// Admin
	http.HandleFunc("/payment-transactions", checkPaymentTransactionsHandler)
	http.HandleFunc("/admin/orders", adminOrdersHandler)
	http.HandleFunc("/admin/fabric-requests", adminFabricRequestsHandler)
	http.HandleFunc("/admin/fabric-request-available", adminFabricRequestAvailableHandler)
	http.HandleFunc("/admin/payment-reports", adminPaymentReportsHandler)
	http.HandleFunc("/admin/confirm-payment", adminConfirmPaymentHandler)
	http.HandleFunc("/admin/customers", adminCustomersHandler)
	http.HandleFunc("/comfort-spoon-control/login", adminLoginHandler)
	http.HandleFunc("/admin/logout", adminLogoutHandler)
	http.HandleFunc("/admin", adminHandler)
	http.HandleFunc("/admin/add-fabric", addFabricHandler)
	http.HandleFunc("/admin/edit-fabric", editFabricHandler)
	http.HandleFunc("/admin/delete-fabric", deleteFabricHandler)
	log.Println("======================================")
	log.Println("🔥 The Comfort Spoon")
	log.Println("🚀 Server running on http://localhost:8080")
	log.Println("======================================")

	err := http.ListenAndServe(":"+getPort(), nil)

	if err != nil {
		log.Fatal(err)
	}
}

// =========================

// =========================
// ADMIN FABRIC REQUESTS
// =========================

func adminFabricRequestsHandler(w http.ResponseWriter, r *http.Request) {

	_, ok := getAdminSession(r)

	if !ok {
		http.Redirect(
			w,
			r,
			"/comfort-spoon-control/login",
			http.StatusSeeOther,
		)
		return
	}

	type FabricRequest struct {
		ID           int
		CustomerName string
		Products     []Product
		Phone        string
		Image        string
		Description  string
		Quantity     string
		Status       string
		Date         string
		WhatsAppURL  string
	}

	rows, err := db.Query(`
		SELECT
			id,
			customer_name,
			phone,
			image,
			description,
			quantity,
			status,
			created_at
		FROM fabric_requests
		ORDER BY id DESC
	`)

	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	defer rows.Close()

	var requests []FabricRequest

	for rows.Next() {

		var f FabricRequest

		err := rows.Scan(
			&f.ID,
			&f.CustomerName,
			&f.Phone,
			&f.Image,
			&f.Description,
			&f.Quantity,
			&f.Status,
			&f.Date,
		)

		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}

		message := fmt.Sprintf("Hello %s 👋\n\nThe Comfort Spoon UPDATE\n\nThe fabric you requested — %s — is now available.\n\nYou can now visit The Comfort Spoon to place your order.\n\nThank you for choosing The Comfort Spoon.", f.CustomerName, f.Description)
		f.WhatsAppURL = "https://wa.me/" + whatsappNumber(f.Phone) + "?text=" + url.QueryEscape(message)

		requests = append(requests, f)
	}

	renderTemplate(
		w,
		"templates/admin_fabric_requests.html",
		requests,
	)
}

func whatsappNumber(phone string) string {
	phone = strings.TrimSpace(phone)
	phone = strings.TrimPrefix(phone, "+")
	if strings.HasPrefix(phone, "0") {
		return "234" + phone[1:]
	}
	if strings.HasPrefix(phone, "234") {
		return phone
	}
	return phone
}

func adminFabricRequestAvailableHandler(w http.ResponseWriter, r *http.Request) {

	_, ok := getAdminSession(r)
	if !ok {
		http.Redirect(w, r, "/comfort-spoon-control/login", http.StatusSeeOther)
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	requestID := r.FormValue("request_id")
	if requestID == "" {
		http.Error(w, "Missing request ID", http.StatusBadRequest)
		return
	}

	_, err := db.Exec(`UPDATE fabric_requests SET status = 'AVAILABLE' WHERE id = ? AND status = 'PENDING'`, requestID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/admin/fabric-requests", http.StatusSeeOther)
}

// ADMIN ORDERS
// =========================

// =========================
// ADMIN REGISTERED CUSTOMERS
// =========================

func adminCustomersHandler(w http.ResponseWriter, r *http.Request) {

	_, loggedIn := getAdminSession(r)

	if !loggedIn {
		http.Redirect(
			w,
			r,
			"/comfort-spoon-control/login",
			http.StatusSeeOther,
		)
		return
	}

	if r.Method != http.MethodGet {
		http.Error(
			w,
			"Method not allowed",
			http.StatusMethodNotAllowed,
		)
		return
	}

	type Customer struct {
		ID    int64
		Name  string
		Phone string
	}

	var customers []Customer

	rows, err := db.Query(`
		SELECT id, full_name, phone
		FROM customers
		ORDER BY full_name COLLATE NOCASE ASC
	`)

	if err != nil {
		http.Error(
			w,
			"Could not load customers: "+err.Error(),
			http.StatusInternalServerError,
		)
		return
	}

	defer rows.Close()

	for rows.Next() {

		var customer Customer

		err := rows.Scan(
			&customer.ID,
			&customer.Name,
			&customer.Phone,
		)

		if err != nil {
			http.Error(
				w,
				"Could not read customer: "+err.Error(),
				http.StatusInternalServerError,
			)
			return
		}

		customers = append(customers, customer)
	}

	if err := rows.Err(); err != nil {
		http.Error(
			w,
			"Could not read customers: "+err.Error(),
			http.StatusInternalServerError,
		)
		return
	}

	renderTemplate(
		w,
		"templates/admin_customers.html",
		customers,
	)
}

func adminOrdersHandler(w http.ResponseWriter, r *http.Request) {

	_, loggedIn := getAdminSession(r)

	if !loggedIn {
		http.Redirect(
			w,
			r,
			"/comfort-spoon-control/login",
			http.StatusSeeOther,
		)
		return
	}

	if r.Method != http.MethodGet {
		http.Error(
			w,
			"Method not allowed",
			http.StatusMethodNotAllowed,
		)
		return
	}

	search := strings.TrimSpace(r.URL.Query().Get("search"))

	type AdminOrder struct {
		ID            int64
		CustomerName  string
		Phone         string
		Total         float64
		AmountPaid    float64
		Balance       float64
		PaymentStatus string
		CreatedAt     string
	}

	type AdminOrdersPage struct {
		Orders            []AdminOrder
		Search            string
		CustomerFound     bool
		CustomerHasOrders bool
		Message           string
	}

	page := AdminOrdersPage{
		Search: search,
	}

	var rows *sql.Rows
	var err error

	if search == "" {

		rows, err = db.Query(`
                        SELECT
                                o.id,
                                c.full_name,
                                c.phone,
                                o.total_amount,
                                o.amount_paid,
                                (
                                        o.total_amount
                                        - o.amount_paid
                                ),
                                o.payment_status,
                                o.created_at
                        FROM orders o
                        JOIN customers c ON c.id = o.customer_id
                        ORDER BY o.id DESC
                `)

		page.CustomerFound = true

	} else {

		var customerID int64

		err = db.QueryRow(`
                        SELECT id
                        FROM customers
                        WHERE full_name LIKE ?
                        ORDER BY id DESC
                        LIMIT 1
                `,
			"%"+search+"%",
		).Scan(&customerID)

		if err == sql.ErrNoRows {
			page.Message = "Customer not found."
			renderTemplate(
				w,
				"templates/admin_orders.html",
				page,
			)
			return
		}

		if err != nil {
			http.Error(
				w,
				"Could not search customer: "+err.Error(),
				http.StatusInternalServerError,
			)
			return
		}

		page.CustomerFound = true

		var orderCount int

		err = db.QueryRow(`
                        SELECT COUNT(*)
                        FROM orders
                        WHERE customer_id = ?
                `, customerID).Scan(&orderCount)

		if err != nil {
			http.Error(
				w,
				"Could not check customer orders: "+err.Error(),
				http.StatusInternalServerError,
			)
			return
		}

		if orderCount == 0 {
			page.Message = "This customer has not placed any orders yet."
			renderTemplate(
				w,
				"templates/admin_orders.html",
				page,
			)
			return
		}

		page.CustomerHasOrders = true

		rows, err = db.Query(`
                        SELECT
                                o.id,
                                c.full_name,
                                c.phone,
                                o.total_amount,
                                o.amount_paid,
                                (
                                        o.total_amount
                                        - o.amount_paid
                                ),
                                o.payment_status,
                                o.created_at
                        FROM orders o
                        JOIN customers c ON c.id = o.customer_id
                        WHERE o.customer_id = ?
                        ORDER BY o.id DESC
                `, customerID)
	}

	if err != nil {
		http.Error(
			w,
			"Could not load orders: "+err.Error(),
			http.StatusInternalServerError,
		)
		return
	}

	defer rows.Close()

	for rows.Next() {

		var order AdminOrder

		err := rows.Scan(
			&order.ID,
			&order.CustomerName,
			&order.Phone,
			&order.Total,
			&order.AmountPaid,
			&order.Balance,
			&order.PaymentStatus,
			&order.CreatedAt,
		)

		if err != nil {
			http.Error(
				w,
				"Could not read orders: "+err.Error(),
				http.StatusInternalServerError,
			)
			return
		}

		parsedTime, parseErr := time.Parse(
			"2006-01-02T15:04:05Z",
			order.CreatedAt,
		)

		if parseErr == nil {
			order.CreatedAt = parsedTime.Format(
				"02 January 2006, 3:04 PM",
			)
		}

		page.Orders = append(page.Orders, order)
	}

	if err := rows.Err(); err != nil {
		http.Error(
			w,
			"Could not read orders: "+err.Error(),
			http.StatusInternalServerError,
		)
		return
	}

	page.CustomerHasOrders = len(page.Orders) > 0

	renderTemplate(
		w,
		"templates/admin_orders.html",
		page,
	)
}
