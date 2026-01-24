# AI Physiotherapist Agent (Go)

> A Go-based AI agent that uses Google Gemini or OpenAI to analyze human posture from images.

## Overview

This project implements an intelligent agent that acts as a physiotherapist. It accepts four images of a person (Front, Left, Right, Back views) and provides a detailed posture analysis, identifying deviations and suggesting corrective exercises.

## Features

- **Multi-view Analysis**: Processes 4 distinct camera angles for comprehensive assessment
- **Multi-Provider Support**: Works with Google Gemini or OpenAI GPT-4
- **Clinical Insights**: Muscle imbalances, compensation chains, priority areas
- **Visual Annotations**: Key findings marked on lateral view image
- **Exercise Recommendations**: Auto-generated corrective exercises
- **Web UI**: Upload images and view reports in browser

## Prerequisites

- Go 1.21+
- Google Gemini API Key or OpenAI API Key

## Installation

1. **Clone the repository**

2. **Install dependencies**:
   ```bash
   go mod tidy
   ```

3. **Configure Environment**:
   ```bash
   cp .env.example .env
   ```
   
   Edit `.env` and add your API key:
   ```
   GOOGLE_API_KEY=your_gemini_key_here
   # or
   OPENAI_API_KEY=your_openai_key_here
   AI_PROVIDER=gemini  # or "openai"
   ```

## Usage

### Command Line

```bash
# Build
go build -o posture_analyzer main.go

# Run with Gemini (default)
./posture_analyzer --front f.jpg --left l.jpg --right r.jpg --back b.jpg

# Run with specific model
./posture_analyzer --model gemini-2.5-pro --front f.jpg --left l.jpg --right r.jpg --back b.jpg

# Run with OpenAI
./posture_analyzer --provider openai --model gpt-4o --front f.jpg --left l.jpg --right r.jpg --back b.jpg
```

### Web UI

```bash
cd web
go build -o server .
./server
```

Open http://localhost:8080 to upload images and view reports.

## Configuration

All settings in `.env`:

| Variable | Description | Default |
|----------|-------------|---------|
| `GOOGLE_API_KEY` | Gemini API key | - |
| `OPENAI_API_KEY` | OpenAI API key | - |
| `AI_PROVIDER` | `gemini` or `openai` | gemini |
| `GEMINI_MODEL` | Gemini model name | gemini-3-flash-preview |
| `OPENAI_MODEL` | OpenAI model name | gpt-4o-mini |
| `OUTPUT_DIR` | Output directory | output |
| `PROMPT_FILE` | Prompt file path | prompt.txt |
| `TEMPLATE_FILE` | HTML template path | template.html |

## Output

Each analysis creates a timestamped folder in `output/` containing:
- `analysis.json` - Raw analysis data
- `report.html` - Visual HTML report
- Copied input images

## Report Sections

1. **Posture Score** - Overall score out of 100
2. **Scan Analysis** - Annotated lateral view with findings
3. **Detailed Zone Analysis** - 6 body regions with metrics
4. **Clinical Insights** - Muscle imbalances, compensation chain, priorities
5. **Exercise Recommendations** - Personalized corrective exercises

## Supported Models

**Gemini:**
- gemini-3-flash-preview (recommended)
- gemini-2.5-pro
- gemini-2.5-flash

**OpenAI:**
- gpt-4o
- gpt-4o-mini
- gpt-4.1

## API Keys

- **Gemini**: https://makersuite.google.com/app/apikey (free tier available)
- **OpenAI**: https://platform.openai.com/api-keys (paid only)

## License

See [LICENSE](LICENSE) file.
