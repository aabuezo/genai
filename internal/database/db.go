package database

import (
	"database/sql"
	"fmt"
	"strings"

	_ "github.com/lib/pq"
)

var DB *sql.DB

func InitDB(connStr string) error {
	var err error
	DB, err = sql.Open("postgres", connStr)
	if err != nil {
		return err
	}
	return DB.Ping()
}

// IsQuerySafe checks if the SQL query contains forbidden keywords.
// This is a basic safety check and should be complemented by database-level permissions.
func IsQuerySafe(query string) bool {
	forbidden := []string{"DROP", "DELETE", "UPDATE", "ALTER", "TRUNCATE"}
	upperQuery := strings.ToUpper(query)
	for _, word := range forbidden {
		if strings.Contains(upperQuery, word) {
			return false
		}
	}
	return true
}

func GetSchema() (string, error) {
	query := `
		SELECT table_name, column_name, data_type 
		FROM information_schema.columns 
		WHERE table_schema = 'public' 
		ORDER BY table_name, ordinal_position;
	`
	rows, err := DB.Query(query)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	var schemaBuilder strings.Builder
	currentTable := ""

	for rows.Next() {
		var tableName, columnName, dataType string
		if err := rows.Scan(&tableName, &columnName, &dataType); err != nil {
			return "", err
		}

		if tableName != currentTable {
			if currentTable != "" {
				schemaBuilder.WriteString(")\n")
			}
			schemaBuilder.WriteString(fmt.Sprintf("TABLE %s (\n", tableName))
			currentTable = tableName
		}
		schemaBuilder.WriteString(fmt.Sprintf("  %s %s,\n", columnName, dataType))
	}
	if currentTable != "" {
		schemaBuilder.WriteString(")\n") // Close the last table
	}

	return schemaBuilder.String(), nil
}

// GetTables returns a list of table names in the database
func GetTables() ([]string, error) {
	query := `
		SELECT table_name
		FROM information_schema.tables
		WHERE table_schema = 'public'
		ORDER BY table_name;
	`
	rows, err := DB.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var tableName string
		if err := rows.Scan(&tableName); err != nil {
			return nil, err
		}
		tables = append(tables, tableName)
	}
	return tables, nil
}
