# AI Physiotherapist Agent (Go)

> A Go-based AI agent that uses Google Gemini to analyze human posture from images.

## Overview

This project implements an intelligent agent that acts as a physiotherapist. It accepts four images of a person (Front, Left, Right, Back views) and provides a detailed posture analysis, identifying deviations and suggesting corrective exercises.

## Features

-   **Multi-view Analysis**: Processes 4 distinct camera angles for a comprehensive assessment.
-   **Physiotherapy Expertise**: Uses specialized prompts with Gemini 2.5 Flash to simulate a professional diagnosis.
-   **Go implementation**: Fast and efficient binary using the Google GenAI Go SDK.

## Prerequisites

-   Go 1.21+
-   Google Cloud API Key (with Gemini access)

## Installation

1.  **Clone the repository**.
2.  **Install dependencies**:
    ```bash
    go mod tidy
    ```
3.  **Configure Environment**:
    -   Copy the example environment file:
        ```bash
        cp .env.example .env
        ```
    -   Edit `.env` and add your Google API key:
        ```
        GOOGLE_API_KEY=your_actual_api_key_here
        ```
    -   **Get your API key**: Visit [Google AI Studio](https://makersuite.google.com/app/apikey) to create a free API key for Gemini.

    **Important**: Never commit your `.env` file to version control. It's already included in `.gitignore` for your protection.

## Usage

1.  **Prepare Images**: Ensure you have 4 images.
2.  **Run the Agent**:
    ```bash
    go run main.go --front path/to/f.jpg --left path/to/l.jpg --right path/to/r.jpg --back path/to/b.jpg
    ```

    Or build and run the binary:
    ```bash
    go build -o videoanalytics_go
    ./videoanalytics_go --front path/to/f.jpg ...
    ```
