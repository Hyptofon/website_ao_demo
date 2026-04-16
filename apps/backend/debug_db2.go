//go:build ignore
// +build ignore

// debug_db2.go — one-off script to inspect analytics DB records.
// Run manually: go run debug_db2.go
// Excluded from `go build` and `go test ./...` by the build constraint above.

package main

import (
	"fmt"
	"database/sql"
	_ "modernc.org/sqlite"
)

func main() {
	db, err := sql.Open("sqlite", "./data/analytics.db")
	if err != nil {
		fmt.Println("DB ERR:", err)
		return
	}
	defer db.Close()

	fmt.Println("=== LAST 5 QUERIES ===")
	rows, _ := db.Query("SELECT query_hash, language, response_ms, feedback, created_at FROM queries ORDER BY created_at DESC LIMIT 5")
	for rows.Next() {
		var h, l, t string
		var ms, f int
		rows.Scan(&h, &l, &ms, &f, &t)
		fmt.Printf("hash=%s lang=%s ms=%d fb=%d time=%s\n", h, l, ms, f, t)
	}
	rows.Close()

	fmt.Println("\n=== TOP QUERIES ===")
	tRows, _ := db.Query("SELECT query_hash, COUNT(*) as count FROM queries GROUP BY query_hash ORDER BY count DESC LIMIT 3")
	for tRows.Next() {
		var h string
		var c int
		tRows.Scan(&h, &c)
		fmt.Printf("hash=%s count=%d\n", h, c)
	}
	tRows.Close()

	fmt.Println("\n=== DAILY STATS ===")
	dRows, _ := db.Query("SELECT date(created_at) as d, COUNT(*) FROM queries GROUP BY d")
	for dRows.Next() {
		var d string
		var c int
		dRows.Scan(&d, &c)
		fmt.Printf("date=%s count=%d\n", d, c)
	}
	dRows.Close()

}
