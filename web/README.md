# Posture Analysis Web UI

Simple web interface for the posture analysis tool.

## Quick Start

```bash
# From the web directory
go build -o server .
./server
```

Open http://localhost:8080

## Features

- Upload 4 images (front, back, left, right)
- Automatic analysis using configured AI provider
- View generated HTML report
- Reports saved to `output/` directory

## Configuration

Uses the same `.env` file as the main application (looks in parent directory):

```
GOOGLE_API_KEY=your_key
AI_PROVIDER=gemini
GEMINI_MODEL=gemini-3-flash-preview
```

## Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/` | GET | Upload form |
| `/analyze` | POST | Submit images for analysis |
| `/report/{id}` | GET | View generated report |

## Port

Default: `8080`

Override with `PORT` environment variable:
```bash
PORT=3000 ./server
```
