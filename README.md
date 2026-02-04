# Asana Extractor

A production-grade Go CLI tool that periodically extracts users, projects, and tasks from the Asana API and saves them as JSON files locally. Automatically discovers and extracts data from all accessible workspaces.

## Features

- **Multi-Workspace Support**: Automatically discovers and extracts data from all workspaces accessible to your account
- **Complete Data Extraction**: Extracts users, projects, and tasks with full field details
- **Rate Limit Aware**: Respects Asana API rate limits (150/1500 req/min) with token bucket + concurrent request limiting
- **Persistent Rate Limiting**: Saves rate limiter state to disk, preventing rate limit violations across restarts
- **Worker Pool**: Configurable concurrency with automatic pool size calculation based on rate limits
- **Graceful Shutdown**: SIGINT/SIGTERM handlers ensure in-flight requests complete before exit
- **Structured Logging**: JSON-formatted logs via Go's `slog` package
- **Pagination**: Handles large workspaces with automatic pagination
- **CLI Driven**: Simple command-line interface for configuration
- **Testable**: Comprehensive unit and integration tests

## Prerequisites

- Go 1.21 or later
- Asana account with a personal access token (PAT)

## Setup

1. **Clone the repository**
   ```bash
   cd /home/marcel/Documents/projects/temp/asana-extractor
   ```

2. **Create `.env` file**
   ```bash
   cp .env.example .env
   ```
   
   Edit `.env` and add your Asana personal access token:
   ```
   ASANA_TOKEN=your_token_here
   ```

3. **Install dependencies** (if not using vendored dependencies)
   ```bash
   go mod download
   ```

## Usage

### Basic Usage (5-minute interval, auto-calculated worker pool)
```bash
go run main.go
```

### Custom Interval (5 minutes or 30 seconds)
```bash
go run main.go --interval 30s
go run main.go --interval 5m
```

### Custom Output Directory
```bash
go run main.go --output-dir ./my_data
```

### Custom Worker Pool Size (overrides auto-calculation)
```bash
go run main.go --workers 20
```

### Custom Log Level
```bash
go run main.go --log-level debug
```

### All Options Combined
```bash
go run main.go --interval 30s --output-dir ./my_data --workers 15 --log-level info
```

## Building

```bash
go build -o asana-extractor main.go
./asana-extractor --interval 5m
```

## Output

Extracted data is saved to the output directory organized by workspace, with the following structure:

```
data/
├── rate_limiter_state.json          # Persisted rate limiter state
├── 1234567890123/                   # Workspace GID
│   ├── users/
│   │   ├── user_111111111111.json
│   │   ├── user_222222222222.json
│   │   └── ...
│   ├── projects/
│   │   ├── project_333333333333.json
│   │   ├── project_444444444444.json
│   │   └── ...
│   └── tasks/
│       ├── task_555555555555.json
│       ├── task_666666666666.json
│       └── ...
└── 9876543210987/                   # Another workspace GID
    ├── users/
    │   └── ...
    ├── projects/
    │   └── ...
    └── tasks/
        └── ...
```

Each JSON file contains a single object with full Asana API response fields including:
- **Users**: GID, name, email, photo, workspaces, timestamps
- **Projects**: GID, name, owner, team, description, status, members, followers, custom fields, timestamps
- **Tasks**: GID, name, assignee, completion status, due dates, notes, subtasks, parent, projects, tags, followers, custom fields, timestamps

## Configuration

### Environment Variables

- `ASANA_TOKEN` (required): Your Asana personal access token
- `EXTRACTION_INTERVAL` (optional): How often to extract data (default: `5m`)
- `OUTPUT_DIR` (optional): Where to save JSON files (default: `./data`)
- `WORKER_COUNT` (optional): Number of concurrent workers (default: auto-calculated)
- `LOG_LEVEL` (optional): Logging level - debug, info, warn, error (default: `info`)

### CLI Flags

- `--interval`: Extraction interval (5m or 30s)
- `--output-dir`: Output directory path
- `--workers`: Number of concurrent workers
- `--log-level`: Logging level

CLI flags override environment variables.

## Rate Limiting

The tool respects Asana's rate limits:

- **Standard Rate Limit**: 150 requests/minute (free tier), 1500 requests/minute (paid tier)
- **Concurrent Limit**: 50 concurrent GET requests, 15 concurrent write requests
- **Token Bucket Algorithm**: Implements token bucket for smooth rate limiting
- **Retry-After Header**: Respects server-provided retry delays
- **State Persistence**: Rate limiter state is saved to `data/rate_limiter_state.json` and restored on restart, preventing rate limit violations when restarting the application

Worker pool size is automatically calculated as: `min(concurrent_limit, estimated_requests_per_cycle / avg_requests_per_item)`, with a conservative default of 30 workers.

## Graceful Shutdown

When you send SIGINT (Ctrl+C) or SIGTERM, the tool will:

1. Stop accepting new extraction tasks
2. Wait for in-flight requests to complete
3. Flush remaining data to disk
4. Exit cleanly

## Testing

Run all tests:
```bash
go test ./...
```

Run with coverage:
```bash
go test -cover ./...
```

Run specific package tests:
```bash
go test ./api -v
go test ./limiter -v
go test ./storage -v
go test ./scheduler -v
```

## Architecture

- **main.go**: CLI setup, signal handlers, workspace discovery, scheduler orchestration
- **config/**: Environment and CLI flag parsing
- **models/**: Data structures for Asana objects (User, Project, Task, Workspace)
- **api/**: Asana API client with authentication, pagination, and integrated rate limiting
- **limiter/**: Rate limiting with token bucket, concurrent request tracking, and state persistence
- **storage/**: JSON file persistence organized by workspace
- **scheduler/**: Worker pool orchestration with graceful shutdown

## Error Handling

The tool implements comprehensive error handling:

- Failed API requests trigger automatic retries with exponential backoff
- Rate limit errors are respected with Retry-After delays
- Partially failed extractions are logged and don't halt the process
- File system errors are logged and retried on next cycle

## Scaling Considerations

For workspaces with thousands of projects/employees:

1. **Worker Pool**: Default pool size (30) is conservative. Increase via `--workers` flag for faster extraction
2. **Pagination**: API client automatically handles pagination; no configuration needed
3. **Storage**: JSON files are stored individually per workspace, allowing parallel writes
4. **Memory**: Streaming JSON parsing prevents loading entire responses into memory
5. **Network**: Concurrent requests are rate-limited to API constraints
6. **Rate Limit Persistence**: On restart, the tool accounts for time elapsed and available tokens

Example for large workspace:
```bash
go run main.go --interval 30s --workers 40 --output-dir /mnt/large_workspace_data
```

## How It Works

1. **Startup**: The tool loads configuration from environment variables and CLI flags
2. **Workspace Discovery**: Fetches all workspaces accessible to the authenticated user
3. **Initial Extraction**: Immediately performs the first extraction cycle
4. **Periodic Extraction**: Runs extraction cycles at the configured interval (5m or 30s)
5. **Data Flow**: For each workspace:
   - Fetches all users and saves to `{workspace_gid}/users/`
   - Fetches all projects and saves to `{workspace_gid}/projects/`
   - For each project, fetches all tasks and saves to `{workspace_gid}/tasks/`
6. **Rate Limiting**: All API calls go through the rate limiter; state is persisted after each call
7. **Shutdown**: On SIGINT/SIGTERM, completes in-flight requests and exits cleanly

## License

MIT
