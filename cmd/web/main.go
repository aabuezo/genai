package main

import (
	"archive/zip"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	"genai/internal/database"
	"genai/internal/gemini"

	_ "github.com/lib/pq"
)

type Application struct {
	DB     *sql.DB
	Gemini *gemini.Client
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "4000"
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL is required")
	}

	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		log.Fatal("GEMINI_API_KEY is required")
	}

	if err := database.InitDB(dbURL); err != nil {
		log.Fatal(err)
	}
	defer database.DB.Close()

	modelName := os.Getenv("GEMINI_MODEL")
	geminiClient, err := gemini.NewClient(apiKey, modelName)
	if err != nil {
		log.Fatal(err)
	}
	defer geminiClient.Close()

	app := &Application{
		DB:     database.DB,
		Gemini: geminiClient,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", app.home)
	mux.HandleFunc("/upload-ddl", app.uploadDDL)
	mux.HandleFunc("/generate-data", app.generateData)
	mux.HandleFunc("/query", app.query)
	mux.HandleFunc("/list-tables", app.listTables)
	mux.HandleFunc("/download-csv", app.downloadCSV)
	mux.HandleFunc("/download-zip", app.downloadZip)

	log.Printf("Starting server on :%s", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatal(err)
	}
}

func (app *Application) home(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	ts, err := template.ParseFiles("ui/html/index.html")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	err = ts.Execute(w, nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (app *Application) uploadDDL(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "Invalid file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, "Error reading file", http.StatusInternalServerError)
		return
	}

	sqlContent := string(content)
	// Basic safety check for creating tables is relaxed as per requirements,
	// but we should still ensure it's a DDL.
	// For this prototype, we trust the DDL input but catch execution errors.

	_, err = app.DB.Exec(sqlContent)
	if err != nil {
		http.Error(w, fmt.Sprintf("Database error: %v", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Schema applied successfully"))
}

func (app *Application) generateData(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Temperature float32 `json:"temperature"`
		MaxTokens   int     `json:"maxTokens"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	schema, err := database.GetSchema()
	if err != nil {
		http.Error(w, "Error fetching schema", http.StatusInternalServerError)
		return
	}

	if schema == "" {
		http.Error(w, "No tables found in database", http.StatusBadRequest)
		return
	}

	sqlResult, err := app.Gemini.GenerateDataSQL(r.Context(), schema, req.Temperature, req.MaxTokens)
	if err != nil {
		http.Error(w, fmt.Sprintf("Gemini error: %v", err), http.StatusInternalServerError)
		return
	}

	// Execute generated SQL
	// Split by semicolon to handle multiple statements if Gemini returns them
	statements := strings.Split(sqlResult, ";")
	tx, err := app.DB.Begin()
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	for _, stmt := range statements {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if _, err := tx.Exec(stmt); err != nil {
			tx.Rollback()
			http.Error(w, fmt.Sprintf("Error executing generated SQL: %v\nSQL: %s", err, stmt), http.StatusInternalServerError)
			return
		}
	}

	if err := tx.Commit(); err != nil {
		http.Error(w, "Transaction commit error", http.StatusInternalServerError)
		return
	}

	// Return the data for the first table found (as a preview)
	tables, _ := database.GetTables()
	if len(tables) == 0 {
		w.Write([]byte("Data generated but no tables found to preview"))
		return
	}

	// Fetch preview data for the first table
	previewData, err := app.fetchingTableData(tables[0])
	if err != nil {
		// Just verify success if we can't fetch preview
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Data generated successfully"))
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "Data generated successfully",
		"preview": previewData,
		"table":   tables[0],
	})
}

func (app *Application) query(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed) // Fixed 405 error
		return
	}

	var req struct {
		Prompt string `json:"prompt"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	schema, err := database.GetSchema()
	if err != nil {
		http.Error(w, "Error fetching schema", http.StatusInternalServerError)
		return
	}

	generatedSQL, isChart, err := app.Gemini.NaturalLanguageToSQL(r.Context(), schema, req.Prompt)
	if err != nil {
		http.Error(w, fmt.Sprintf("AI Error: %v", err), http.StatusInternalServerError)
		return
	}

	// Remove Chart comment for execution
	execSQL := generatedSQL
	chartType := ""
	if isChart {
		parts := strings.Split(generatedSQL, "-- CHART:")
		if len(parts) > 1 {
			execSQL = parts[0]
			chartType = strings.TrimSpace(parts[1])
		}
	}

	if !database.IsQuerySafe(execSQL) {
		http.Error(w, "Unsafe query generated. Operation blocked.", http.StatusForbidden)
		return
	}

	rows, err := app.DB.Query(execSQL)
	if err != nil {
		http.Error(w, fmt.Sprintf("Query execution error: %v\nSQL: %s", err, execSQL), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	cols, _ := rows.Columns()
	var result []map[string]interface{}

	for rows.Next() {
		columns := make([]interface{}, len(cols))
		columnPointers := make([]interface{}, len(cols))
		for i := range columns {
			columnPointers[i] = &columns[i]
		}

		if err := rows.Scan(columnPointers...); err != nil {
			continue
		}

		m := make(map[string]interface{})
		for i, colName := range cols {
			val := columnPointers[i].(*interface{})
			m[colName] = *val
		}
		result = append(result, m)
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"sql":       generatedSQL,
		"result":    result,
		"isChart":   isChart,
		"chartType": chartType,
	})
}

func (app *Application) downloadCSV(w http.ResponseWriter, r *http.Request) {
	tableName := r.URL.Query().Get("table")
	if tableName == "" {
		// Default to first table if not specified
		tables, _ := database.GetTables()
		if len(tables) > 0 {
			tableName = tables[0]
		} else {
			http.Error(w, "No table specified", http.StatusBadRequest)
			return
		}
	}

	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s.csv", tableName))

	rows, err := app.DB.Query(fmt.Sprintf("SELECT * FROM %s", tableName))
	if err != nil {
		http.Error(w, "Error querying table", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	csvWriter := csv.NewWriter(w)
	defer csvWriter.Flush()

	cols, _ := rows.Columns()
	csvWriter.Write(cols)

	for rows.Next() {
		columns := make([]interface{}, len(cols))
		columnPointers := make([]interface{}, len(cols))
		for i := range columns {
			columnPointers[i] = &columns[i]
		}

		if err := rows.Scan(columnPointers...); err != nil {
			continue
		}

		record := make([]string, len(cols))
		for i, val := range columns {
			if val == nil {
				record[i] = ""
			} else {
				record[i] = fmt.Sprintf("%v", val)
			}
		}
		csvWriter.Write(record)
	}
}

func (app *Application) downloadZip(w http.ResponseWriter, r *http.Request) {
	tables, err := database.GetTables()
	if err != nil {
		http.Error(w, "Error fetching tables", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", "attachment; filename=all_data.zip")

	zipWriter := zip.NewWriter(w)
	defer zipWriter.Close()

	for _, tableName := range tables {
		rows, err := app.DB.Query(fmt.Sprintf("SELECT * FROM %s", tableName))
		if err != nil {
			continue
		}

		f, err := zipWriter.Create(tableName + ".csv")
		if err != nil {
			rows.Close()
			continue
		}

		csvWriter := csv.NewWriter(f)
		cols, _ := rows.Columns()
		csvWriter.Write(cols)

		for rows.Next() {
			columns := make([]interface{}, len(cols))
			columnPointers := make([]interface{}, len(cols))
			for i := range columns {
				columnPointers[i] = &columns[i]
			}

			rows.Scan(columnPointers...)
			record := make([]string, len(cols))
			for i, val := range columns {
				if val == nil {
					record[i] = ""
				} else {
					record[i] = fmt.Sprintf("%v", val)
				}
			}
			csvWriter.Write(record)
		}
		csvWriter.Flush()
		rows.Close()
	}
}

// Helper to get raw data for preview
func (app *Application) fetchingTableData(tableName string) ([]map[string]interface{}, error) {
	rows, err := app.DB.Query(fmt.Sprintf("SELECT * FROM %s LIMIT 10", tableName))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cols, _ := rows.Columns()
	var result []map[string]interface{}

	for rows.Next() {
		columns := make([]interface{}, len(cols))
		columnPointers := make([]interface{}, len(cols))
		for i := range columns {
			columnPointers[i] = &columns[i]
		}

		if err := rows.Scan(columnPointers...); err != nil {
			continue
		}

		m := make(map[string]interface{})
		for i, colName := range cols {
			val := columnPointers[i].(*interface{})
			if val != nil {
				// Handle []uint8 explicitly for strings in some drivers,
				// though lib/pq usually handles standard types well.
				// For simplicity in JSON, let fmt handle formatting
				m[colName] = fmt.Sprintf("%v", *val)
			} else {
				m[colName] = nil
			}
		}
		result = append(result, m)
	}
	return result, nil
}

func (app *Application) listTables(w http.ResponseWriter, r *http.Request) {
	tables, err := database.GetTables()
	if err != nil {
		http.Error(w, "Error fetching tables", http.StatusInternalServerError)
		return
	}

	type TableInfo struct {
		Name string                   `json:"name"`
		Data []map[string]interface{} `json:"data"`
	}

	var result []TableInfo

	for _, tableName := range tables {
		data, err := app.fetchingTableData(tableName)
		if err != nil {
			continue // Skip tables with errors
		}
		result = append(result, TableInfo{
			Name: tableName,
			Data: data,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}
