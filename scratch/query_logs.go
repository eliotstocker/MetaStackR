package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	db, err := sql.Open("pgx", "postgresql://neondb_owner:npg_e7HbYKfiFD9y@ep-quiet-glade-za992ct6-pooler.c-2.eu-west-2.aws.neon.tech/neondb?sslmode=require")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	rows, err := db.QueryContext(context.Background(), "SELECT id, event_type, payload, created_at FROM merge_audit_logs ORDER BY id DESC LIMIT 10")
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()

	for rows.Next() {
		var id int64
		var eventType, payload, createdAt string
		if err := rows.Scan(&id, &eventType, &payload, &createdAt); err != nil {
			log.Fatal(err)
		}
		fmt.Printf("[%d] %s: %s (%s)\n", id, eventType, payload, createdAt)
	}
}
