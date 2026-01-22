# Go AI Monolith - Data Assistant

This is a monolithic web application built with **Go (Golang)** that leverages **Google Gemini AI** to assist with database operations. It provides an intuitive interface for generating massive amounts of synthetic data from schema definitions and querying that data using natural language.

![Data Assistant UI](ui/html/screenshot_placeholder.png)

## Features

### 1. Data Generation
-   **Schema Parsing**: Upload any PostgreSQL `.ddl` file. The system automatically creates the tables in the database.
-   **AI-Powered Generation**: Uses Gemini (gemini-1.5-flash) to generate context-aware `INSERT` statements based on your schema.
-   **Customizable**: Adjust **Temperature** (creativity) and **Max Tokens** to control the variety and volume of generated data.
-   **Real-time Preview**: View a sample of the generated data immediately.

### 2. Talk to your Data
-   **Natural Language Queries**: Ask questions like *"Show me the top 5 customers by spending"* or *"List all orders from yesterday"*.
-   **Automatic SQL**: The AI converts your questions into safe, read-only SQL queries (`SELECT` only).
-   **Visualization**: Ask for charts (e.g., *"Show a bar chart of sales by region"*) to automatically render visualizations using Chart.js.

### 3. Export
-   **Download Data**: Export your tables as CSV files or download the entire database as a ZIP archive.

## Prerequisites

-   **Docker** & **Docker Compose**
-   A **Google Gemini API Key** (Get one at [aistudio.google.com](https://aistudio.google.com/))

## Quick Start

1.  **Clone the repository**:
    ```bash
    git clone <repository-url>
    cd genai
    ```

2.  **Set your API Key**:
    You can export it as an environment variable or create a `.env` file (if you add support for it). For Docker Compose, passing it directly works best:
    ```bash
    export GEMINI_API_KEY="your_actual_api_key_here"
    ```

3.  **Run with Docker Compose**:
    This command builds the Go application and starts the PostgreSQL database.
    ```bash
    docker-compose up --build
    ```

4.  **Access the App**:
    Open your browser and navigate to:
    [http://localhost:4000](http://localhost:4000)

## Configuration

The application is configured via environment variables.

| Variable | Description | Default (in Docker) |
| :--- | :--- | :--- |
| `GEMINI_API_KEY` | **Required**. Your Google AI API Key. | None |
| `GEMINI_MODEL` | Gemini model to use. | `gemini-2.0-flash` |
| `DATABASE_URL` | Connection string for PostgreSQL. | `postgres://user:password@db:5432/genai?sslmode=disable` |
| `PORT` | Port for the web server. | `4000` |

## Development Workflow

### Rebuilding After Code Changes

When you make changes to the Go source code, you need to rebuild the Docker container:

```bash
# Set your environment variables
export GEMINI_API_KEY="your_actual_api_key_here"
export GEMINI_MODEL="gemini-2.0-flash"

# Stop, remove, and rebuild the app container
docker compose stop app && docker compose rm -f app && docker compose up --build -d app
```

**Note**: Changes to `ui/html/index.html` only require a browser refresh, not a rebuild.

### Available Gemini Models

To see which models are available with your API key, run:

```bash
GEMINI_API_KEY="your_key" go run cmd/list_models/main.go
```

Common models for the free tier:
- `gemini-2.0-flash` (recommended, default)
- `gemini-2.5-flash`
- `gemini-flash-latest`


## Project Structure

```
├── cmd/
│   └── web/
│       └── main.go       # Application entry point & HTTP handlers
├── internal/
│   ├── database/         # Database connection & safety logic
│   └── gemini/           # AI Client & System Instructions
├── ui/
│   └── html/
│       └── index.html    # Single-page UI (Tailwind + Vanilla JS)
├── Dockerfile            # Multi-stage build for Go
├── docker-compose.yml    # App + Postgres orchestration
└── go.mod                # Go dependencies
```

## Security Note

-   **Prompt Injection**: The system uses a restricted regex blocklist (`DROP`, `DELETE`, `UPDATE`, `TRUNCATE`) to prevent destructive queries.
-   **System Instructions**: The AI is instructed via strictly scoped system prompts to only perform "Read" operations in the analysis mode.
-   **Environment**: It is recommended to run this in a development or sandboxed environment.
