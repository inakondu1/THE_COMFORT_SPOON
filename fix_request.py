from pathlib import Path

p = Path("main.go")
text = p.read_text()

start = text.find('_, err = db.Exec(`\n\t\tINSERT INTO fabric_requests')

if start == -1:
    print("START NOT FOUND")
    exit()

end = text.find('\n\t\tif err != nil {', start)

if end == -1:
    print("END NOT FOUND")
    exit()

new = '''var customerName string
        var phone string

        err = db.QueryRow(`
                SELECT full_name, phone
                FROM customers
                WHERE id = ?
        `, customerID).Scan(&customerName, &phone)

        if err != nil {
                http.Error(w, "Could not load customer details: "+err.Error(), http.StatusInternalServerError)
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

'''

text = text[:start] + new + text[end:]

p.write_text(text)

print("✅ Fabric request insert fixed")
