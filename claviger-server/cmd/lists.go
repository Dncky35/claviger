package cmd

import (
	"claviger-server/storage"
	"database/sql"
	"fmt"
	"log"
	"os"
	"text/tabwriter"
)

func RunGetList(args []string) {
	db := storage.InitDB()
	defer db.Close()

	query := "SELECT id, name, public_key, ip_address, role_id, status FROM clients"
	var err error
	var rows *sql.Rows

	// Filter by role_id if an argument is provided
	if len(args) > 0 {
		roleFilter := args[0]
		query += " WHERE role_id = ? ORDER BY created_at DESC"
		rows, err = db.Query(query, roleFilter)
	} else {
		query += " ORDER BY created_at DESC"
		rows, err = db.Query(query)
	}

	if err != nil {
		log.Fatalf("❌ Failed to fetch clients: %v", err)
	}
	defer rows.Close()

	// Initialize tabwriter for beautifully aligned console output
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "ID\tNAME\tROLE\tIP ADDRESS\tSTATUS\tPUBLIC KEY")
	fmt.Fprintln(w, "--\t----\t----\t----------\t------\t----------")

	var count int
	for rows.Next() {
		var id, name, pubKey, ip, role, status string
		if err := rows.Scan(&id, &name, &pubKey, &ip, &role, &status); err != nil {
			log.Printf("⚠️ Error reading row: %v", err)
			continue
		}

		// Truncate public key for cleaner display
		displayKey := pubKey
		if len(pubKey) > 10 {
			displayKey = pubKey[:10] + "..."
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", id, name, role, ip, status, displayKey)
		count++
	}

	w.Flush()
	fmt.Printf("\nTotal Clients: %d\n", count)
}
