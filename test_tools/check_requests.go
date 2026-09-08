package main

import (
	"fmt"
)

func main() {
	initDatabase()

	rows, err := db.Query(`
		SELECT id, customer_id, customer_name, phone, description, quantity, status
		FROM fabric_requests
	`)

	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	defer rows.Close()

	for rows.Next() {
		var id int
		var customerID int
		var name string
		var phone string
		var description string
		var quantity int
		var status string

		rows.Scan(
			&id,
			&customerID,
			&name,
			&phone,
			&description,
			&quantity,
			&status,
		)

		fmt.Println(
			id,
			customerID,
			name,
			phone,
			description,
			quantity,
			status,
		)
	}
}
