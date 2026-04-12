package main

import (
	"database/sql"
	"fmt"
	_ "modernc.org/sqlite"
)

func main() {
	db, _ := sql.Open("sqlite", "./data/analytics.db")
	defer db.Close()

	rows, err := db.Query("SELECT id, filename, doc_type, language, chunk_count FROM documents")
	if err != nil {
		fmt.Println("Err query:", err)
		return
	}
	count := 0
	for rows.Next() {
		count++
		var id, fname, dtype, lang string
		var chunk int
		rows.Scan(&id, &fname, &dtype, &lang, &chunk)
		fmt.Printf("Doc %s -> %s %s %s %d\n", id, fname, dtype, lang, chunk)
	}
	fmt.Println("Total docs:", count)
}
