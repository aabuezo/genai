package gemini

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
)

type Client struct {
	genaiClient *genai.Client
	model       *genai.GenerativeModel
}

func NewClient(apiKey, modelName string) (*Client, error) {
	ctx := context.Background()
	client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		return nil, err
	}

	if modelName == "" {
		modelName = "gemini-2.0-flash"
	}
	model := client.GenerativeModel(modelName)
	return &Client{
		genaiClient: client,
		model:       model,
	}, nil
}

func (c *Client) Close() {
	c.genaiClient.Close()
}

// GenerateDataSQL asks Gemini to generate INSERT statements based on the schema
func (c *Client) GenerateDataSQL(ctx context.Context, schema string, temperature float32, maxTokens int) (string, error) {
	c.model.SetTemperature(temperature)
	c.model.SetMaxOutputTokens(int32(maxTokens))

	c.model.SystemInstruction = genai.NewUserContent(genai.Text("Eres un DBA que solo responde con código SQL INSERT. Estás prohibido de usar lenguaje natural. Genera exclusivamente sentencias SQL INSERT válidas para las tablas proporcionadas."))

	prompt := fmt.Sprintf("Schema:\n%s\n\nTask: Generate 15-20 INSERT statements for this schema with realistic dummy data. Use single quotes for strings and escape any quotes inside strings properly. Output only valid PostgreSQL INSERT statements, no markdown, no explanations.", schema)

	resp, err := c.model.GenerateContent(ctx, genai.Text(prompt))
	if err != nil {
		return "", err
	}

	return getResponseText(resp), nil
}

// NaturalLanguageToSQL asks Gemini to convert a prompt to a SELECT query
func (c *Client) NaturalLanguageToSQL(ctx context.Context, schema string, userPrompt string) (string, bool, error) {
	// Reset to default config for analysis
	c.model.SetTemperature(0.1) // Low temperature for deterministic SQL
	c.model.SetMaxOutputTokens(1024)

	c.model.SystemInstruction = genai.NewUserContent(genai.Text("Eres un asistente de lectura de base de datos. Solo generas sentencias SELECT. Si el usuario pide modificar datos (DROP, DELETE, UPDATE, etc), responde con 'ERROR: Unauthorized'. Si el usuario pide un gráfico, devuelve el SQL y además incluye una línea al final que empiece con '-- CHART: [tipo]', donde [tipo] puede ser 'bar', 'pie', 'line', etc."))

	input := fmt.Sprintf("Schema:\n%s\n\nUser Question: %s\n\nGenerate the SQL query:", schema, userPrompt)

	resp, err := c.model.GenerateContent(ctx, genai.Text(input))
	if err != nil {
		return "", false, err
	}

	text := getResponseText(resp)

	// Clean up markdown code blocks if present
	text = strings.TrimPrefix(text, "```sql")
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSuffix(text, "```")
	text = strings.TrimSpace(text)

	isChart := strings.Contains(text, "-- CHART:")

	return text, isChart, nil
}

func getResponseText(resp *genai.GenerateContentResponse) string {
	if len(resp.Candidates) == 0 || len(resp.Candidates[0].Content.Parts) == 0 {
		return ""
	}

	var sb strings.Builder
	for _, part := range resp.Candidates[0].Content.Parts {
		if txt, ok := part.(genai.Text); ok {
			sb.WriteString(string(txt))
		}
	}
	text := sb.String()

	// Clean up markdown code blocks if present
	text = strings.TrimSpace(text)
	text = strings.TrimPrefix(text, "```sql")
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSpace(text)
	text = strings.TrimSuffix(text, "```")
	text = strings.TrimSpace(text)

	return text
}
