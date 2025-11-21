# Excel Translator (Doubao LLM)

This project is a robust, production-ready tool for translating Excel files using the Doubao (Volcengine) Large Language Model. It is designed with **Clean Architecture** principles and a **Durable Execution** pattern to ensure reliability, maintainability, and fault tolerance.

## Design Philosophy

The core design philosophy revolves around **High Cohesion, Low Coupling, and Deep Testability**.

### 1. Clean Architecture (Hexagonal Architecture)

The application is stratified into three distinct layers, ensuring that business logic is independent of external technologies (UI, Database, APIs).

*   **Domain Layer (`pkg/domain`)**:
    *   Contains the core business entities (`Task`, `Cell`) and interfaces (`Translator`, `TaskRepository`).
    *   This layer has **zero dependencies** on other layers or external libraries. It defines *what* the system does, not *how*.
*   **Service Layer (`pkg/service`)**:
    *   Contains the application logic (`TranslationEngine`).
    *   It orchestrates the flow: Ingest -> Process -> Export.
    *   It relies only on the Domain interfaces, allowing us to swap out implementations (e.g., mock LLM, different database) without changing a line of business logic.
*   **Adapter Layer (`pkg/adapter`)**:
    *   Contains the concrete implementations of the interfaces.
    *   **Doubao Adapter**: Handles the specific JSON protocol, system prompts, and rate limiting for the Volcengine API.
    *   **SQLite Adapter**: Manages persistence using a local SQLite database.
    *   **Excel Adapter**: Handles file I/O and dynamic column detection.

### 2. Durable Execution & Resumability

Translation is a long-running process prone to network failures, API rate limits, or user interruptions. To address this, we treat the translation process as a **State Machine** persisted in a local SQLite database.

*   **State Persistence**: Every row to be translated is ingested as a `Task` in SQLite with a status (`PENDING`, `PROCESSING`, `COMPLETE`, `FAILED`).
*   **Idempotency**: The system calculates a hash of the input file to generate a unique `Job ID`. If you re-run the tool on the same file, it detects existing tasks and resumes exactly where it left off.
*   **Atomic Updates**: Task status updates are transactional, ensuring that we never lose track of a cell's state.

### 3. Resilience & Graceful Shutdown

*   **Signal Handling**: The application listens for `SIGINT` (Ctrl+C) and `SIGTERM`.
*   **Partial Saves**: Upon interruption, the system cancels pending network requests but immediately exports all *completed* tasks to the Excel file. This ensures that 1 hour of processing isn't lost due to a sudden stop.
*   **Rate Limiting**: A Token Bucket rate limiter (via `golang.org/x/time/rate`) enforces strict adherence to the Doubao API's RPM (Requests Per Minute) limits.

### 4. Dynamic Flexibility

*   **Dynamic Headers**: The tool doesn't hardcode target languages. It identifies the `key` and `CH` (source) columns and treats **any other column** as a target language. This allows users to add `ES`, `DE`, `JP` columns to their Excel file and have them translated automatically without code changes.

## Usage

### Prerequisites

*   Go 1.21+
*   Doubao API Key and Endpoint ID

### Environment Variables

Create a `.env` file or set these variables:

```bash
DOUBAO_API_KEY=your_api_key
DOUBAO_ENDPOINT_ID=your_endpoint_id
DOUBAO_API_URL=https://ark.cn-beijing.volces.com/api/v3/chat/completions
DOUBAO_RPM=60  # Requests per minute
```

### Running

1.  **Build**:
    ```bash
    go build -o translator cmd/translator/main.go
    ```

2.  **Run**:
    ```bash
    ./translator input.xlsx output.xlsx
    ```

    *   `input.xlsx`: The source Excel file. Must contain `key` and `CH` headers. Any other headers (e.g., `FR`, `PT`) will be treated as targets.
    *   `output.xlsx`: The destination file.

### Mock Mode (For Testing)

Set `DOUBAO_API_KEY=TEST_KEY` to run in mock mode. This mimics the API latency and response structure without making actual network calls.

```bash
export DOUBAO_API_KEY=TEST_KEY
export DOUBAO_ENDPOINT_ID=test
./translator sample.xlsx output.xlsx
```

## Project Structure

```
├── cmd/
│   └── translator/      # Main entry point, dependency injection wiring
├── pkg/
│   ├── domain/          # Core entities and interfaces (Pure Go)
│   ├── service/         # Business logic (TranslationEngine)
│   └── adapter/         # External implementations
│       ├── doubao/      # LLM API Client
│       ├── excel/       # Excelize wrapper
│       └── sqlite/      # Durable store
```
