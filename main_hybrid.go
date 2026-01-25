package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/google/generative-ai-go/genai"
	"github.com/joho/godotenv"
	openai "github.com/sashabaranov/go-openai"
	"google.golang.org/api/option"
)

type Metric struct {
	Value      float64 `json:"value"`
	Unit       string  `json:"unit"`
	Confidence float64 `json:"confidence"`
}

type PoseMetrics struct {
	CraniovertebralAngle  *Metric `json:"craniovertebral_angle,omitempty"`
	ForwardHeadPosture    *Metric `json:"forward_head_posture,omitempty"`
	ShoulderHeightDelta   *Metric `json:"shoulder_height_delta,omitempty"`
	ThoracicKyphosis      *Metric `json:"thoracic_kyphosis,omitempty"`
	KneeValgusVarus       *Metric `json:"knee_valgus_varus,omitempty"`
	FootProgressionAngle  *Metric `json:"foot_progression_angle,omitempty"`
}

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("Warning: No .env file found")
	}

	frontPtr := flag.String("front", "", "Path to front view image")
	leftPtr := flag.String("left", "", "Path to left view image")
	rightPtr := flag.String("right", "", "Path to right view image")
	backPtr := flag.String("back", "", "Path to back view image")
	heightPtr := flag.Float64("height", 0, "User height in cm (optional, for calibration)")
	providerPtr := flag.String("provider", "gemini", "AI provider: gemini or openai")
	flag.Parse()

	if *frontPtr == "" || *leftPtr == "" || *rightPtr == "" || *backPtr == "" {
		log.Fatal("Error: All 4 views required (--front, --left, --right, --back)")
	}

	// Create output directory first
	outputDir := createOutputDir()

	// Step 1: Extract pose metrics using MediaPipe
	fmt.Println("Step 1: Extracting pose landmarks with MediaPipe...")
	metrics, err := extractPoseMetrics(*frontPtr, *leftPtr, *rightPtr, *backPtr, *heightPtr, outputDir)
	if err != nil {
		log.Fatalf("Error extracting pose: %v", err)
	}

	fmt.Println("\n=== MEASURED METRICS (Deterministic) ===")
	printMetrics(metrics)

	// Step 2: Get LLM interpretation (with images + metrics)
	fmt.Println("\nStep 2: Getting clinical interpretation from LLM (with images + measured data)...")
	interpretation := getLLMInterpretation(metrics, *providerPtr, *frontPtr, *leftPtr, *rightPtr, *backPtr)

	// Step 3: Save results
	saveResults(outputDir, metrics, interpretation, *frontPtr, *leftPtr, *rightPtr, *backPtr)

	fmt.Printf("\n✓ Analysis complete! Results saved to: %s\n", outputDir)
	fmt.Printf("  - analysis.json (combined data)\n")
	fmt.Printf("  - report.html (visual report)\n")
	fmt.Printf("  - metrics.json (measured values)\n")
	fmt.Printf("  - interpretation.json (LLM analysis)\n")
	fmt.Printf("  - *_annotated.jpg (images with landmarks)\n")
}

func extractPoseMetrics(front, left, right, back string, height float64, outputDir string) (*PoseMetrics, error) {
	// Find Python venv
	venvPython := "./venv/bin/python3"
	if _, err := os.Stat(venvPython); os.IsNotExist(err) {
		venvPython = "python3"
	}

	args := []string{"pose_extractor.py", front, left, right, back}
	if height > 0 {
		args = append(args, fmt.Sprintf("%.1f", height))
	}
	// Add output directory for annotated images
	args = append(args, outputDir)

	cmd := exec.Command(venvPython, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("pose extraction failed: %v\n%s", err, output)
	}

	// Parse JSON output
	var result struct {
		Metrics PoseMetrics `json:"metrics"`
	}

	// Filter out stderr logs
	lines := strings.Split(string(output), "\n")
	var jsonLines []string
	inJSON := false
	for _, line := range lines {
		if strings.HasPrefix(line, "{") {
			inJSON = true
		}
		if inJSON {
			jsonLines = append(jsonLines, line)
		}
	}

	jsonStr := strings.Join(jsonLines, "\n")
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return nil, fmt.Errorf("failed to parse metrics: %v", err)
	}

	return &result.Metrics, nil
}

func getLLMInterpretation(metrics *PoseMetrics, provider string, frontPath, leftPath, rightPath, backPath string) string {
	// Load prompt template
	promptTemplate, err := os.ReadFile("prompt_hybrid.txt")
	if err != nil {
		log.Fatalf("Error reading prompt: %v", err)
	}

	// Inject metrics into prompt
	metricsJSON, _ := json.MarshalIndent(metrics, "", "  ")
	prompt := strings.Replace(string(promptTemplate), "{METRICS_JSON}", string(metricsJSON), 1)

	ctx := context.Background()

	if provider == "openai" {
		return runOpenAIHybrid(ctx, prompt, frontPath, leftPath, rightPath, backPath)
	}
	return runGeminiHybrid(ctx, prompt, frontPath, leftPath, rightPath, backPath)
}

func runGeminiHybrid(ctx context.Context, prompt string, frontPath, leftPath, rightPath, backPath string) string {
	apiKey := os.Getenv("GOOGLE_API_KEY")
	if apiKey == "" {
		log.Fatal("Error: GOOGLE_API_KEY not set")
	}

	modelName := os.Getenv("GEMINI_MODEL")
	if modelName == "" {
		modelName = "gemini-2.5-flash-lite"
	}

	client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		log.Fatalf("Error creating Gemini client: %v", err)
	}
	defer client.Close()

	model := client.GenerativeModel(modelName)
	model.SetTemperature(0.0)
	model.SetTopK(1)
	model.SetTopP(0.0)
	model.ResponseMIMEType = "application/json"

	// Build parts with prompt + images + annotated images
	var parts []genai.Part
	parts = append(parts, genai.Text(prompt))

	// Add images
	addImage := func(label, path string) {
		data, err := os.ReadFile(path)
		if err != nil {
			return
		}
		mimeType := "jpeg"
		if strings.HasSuffix(strings.ToLower(path), ".png") {
			mimeType = "png"
		}
		parts = append(parts, genai.Text(fmt.Sprintf("\n[%s]", label)))
		parts = append(parts, genai.ImageData(mimeType, data))
	}

	addImage("Front View", frontPath)
	addImage("Left Side View", leftPath)
	addImage("Right Side View", rightPath)
	addImage("Back View", backPath)

	fmt.Printf("Sending to Gemini (%s) with images + measured data...\n", modelName)
	resp, err := model.GenerateContent(ctx, parts...)
	if err != nil {
		log.Fatalf("Error generating content: %v", err)
	}

	var result strings.Builder
	for _, cand := range resp.Candidates {
		if cand.Content != nil {
			for _, part := range cand.Content.Parts {
				if txt, ok := part.(genai.Text); ok {
					result.WriteString(string(txt))
				}
			}
		}
	}
	return result.String()
}

func runOpenAIHybrid(ctx context.Context, prompt string, frontPath, leftPath, rightPath, backPath string) string {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		log.Fatal("Error: OPENAI_API_KEY not set")
	}

	modelName := os.Getenv("OPENAI_MODEL")
	if modelName == "" {
		modelName = "gpt-4o-mini"
	}

	client := openai.NewClient(apiKey)

	// Encode images to base64
	encodeImage := func(path string) string {
		data, err := os.ReadFile(path)
		if err != nil {
			return ""
		}
		return base64.StdEncoding.EncodeToString(data)
	}

	// Build message with images
	messages := []openai.ChatCompletionMessage{
		{
			Role: openai.ChatMessageRoleUser,
			MultiContent: []openai.ChatMessagePart{
				{Type: openai.ChatMessagePartTypeText, Text: prompt},
				{Type: openai.ChatMessagePartTypeImageURL, ImageURL: &openai.ChatMessageImageURL{
					URL: "data:image/jpeg;base64," + encodeImage(frontPath),
				}},
				{Type: openai.ChatMessagePartTypeImageURL, ImageURL: &openai.ChatMessageImageURL{
					URL: "data:image/jpeg;base64," + encodeImage(leftPath),
				}},
				{Type: openai.ChatMessagePartTypeImageURL, ImageURL: &openai.ChatMessageImageURL{
					URL: "data:image/jpeg;base64," + encodeImage(rightPath),
				}},
				{Type: openai.ChatMessagePartTypeImageURL, ImageURL: &openai.ChatMessageImageURL{
					URL: "data:image/jpeg;base64," + encodeImage(backPath),
				}},
			},
		},
	}

	resp, err := client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model:          modelName,
		Messages:       messages,
		Temperature:    0.0,
		ResponseFormat: &openai.ChatCompletionResponseFormat{Type: openai.ChatCompletionResponseFormatTypeJSONObject},
	})

	if err != nil {
		log.Fatalf("Error calling OpenAI: %v", err)
	}

	return resp.Choices[0].Message.Content
}

func printMetrics(m *PoseMetrics) {
	if m.CraniovertebralAngle != nil {
		fmt.Printf("  Craniovertebral Angle: %.1f° (confidence: %.2f)\n",
			m.CraniovertebralAngle.Value, m.CraniovertebralAngle.Confidence)
	}
	if m.ForwardHeadPosture != nil {
		fmt.Printf("  Forward Head Posture: %.1f%s (confidence: %.2f)\n",
			m.ForwardHeadPosture.Value, m.ForwardHeadPosture.Unit, m.ForwardHeadPosture.Confidence)
	}
	if m.ShoulderHeightDelta != nil {
		fmt.Printf("  Shoulder Height Delta: %.1f%s (confidence: %.2f)\n",
			m.ShoulderHeightDelta.Value, m.ShoulderHeightDelta.Unit, m.ShoulderHeightDelta.Confidence)
	}
	if m.ThoracicKyphosis != nil {
		fmt.Printf("  Thoracic Kyphosis: %.1f° (confidence: %.2f)\n",
			m.ThoracicKyphosis.Value, m.ThoracicKyphosis.Confidence)
	}
	if m.KneeValgusVarus != nil {
		fmt.Printf("  Knee Valgus/Varus: %.1f° (confidence: %.2f)\n",
			m.KneeValgusVarus.Value, m.KneeValgusVarus.Confidence)
	}
	if m.FootProgressionAngle != nil {
		fmt.Printf("  Foot Progression Angle: %.1f° (confidence: %.2f)\n",
			m.FootProgressionAngle.Value, m.FootProgressionAngle.Confidence)
	}
}

func createOutputDir() string {
	outputBase := os.Getenv("OUTPUT_DIR")
	if outputBase == "" {
		outputBase = "output"
	}
	timestamp := time.Now().Format("2006-01-02_15-04-05")
	outputDir := filepath.Join(outputBase, timestamp)
	os.MkdirAll(outputDir, 0755)
	return outputDir
}

func saveResults(outputDir string, metrics *PoseMetrics, interpretation string, frontPath, leftPath, rightPath, backPath string) {
	// Save metrics
	metricsFile := filepath.Join(outputDir, "metrics.json")
	metricsJSON, _ := json.MarshalIndent(metrics, "", "  ")
	os.WriteFile(metricsFile, metricsJSON, 0644)

	// Save interpretation
	interpFile := filepath.Join(outputDir, "interpretation.json")
	os.WriteFile(interpFile, []byte(interpretation), 0644)

	// Parse interpretation for HTML report
	var interpData map[string]interface{}
	json.Unmarshal([]byte(interpretation), &interpData)

	// Build Regions array from body sections for template
	buildRegionsAndScore(interpData)

	// Copy images to output directory
	copyImage(frontPath, filepath.Join(outputDir, "front"+filepath.Ext(frontPath)))
	copyImage(leftPath, filepath.Join(outputDir, "left"+filepath.Ext(leftPath)))
	copyImage(rightPath, filepath.Join(outputDir, "right"+filepath.Ext(rightPath)))
	copyImage(backPath, filepath.Join(outputDir, "back"+filepath.Ext(backPath)))

	// Add input_images map for template (use annotated versions)
	interpData["input_images"] = map[string]string{
		"front": "front_annotated.jpg",
		"left":  "left_annotated.jpg",
		"right": "right_annotated.jpg",
		"back":  "back_annotated.jpg",
	}

	// Save analysis.json (flattened for template compatibility)
	analysisFile := filepath.Join(outputDir, "analysis.json")
	analysisJSON, _ := json.MarshalIndent(interpData, "", "  ")
	os.WriteFile(analysisFile, analysisJSON, 0644)

	// Generate HTML report
	generateHTMLReport(outputDir, interpData)
}

func copyImage(src, dst string) {
	data, err := os.ReadFile(src)
	if err == nil {
		os.WriteFile(dst, data, 0644)
	}
}

func buildRegionsAndScore(analysisData map[string]interface{}) {
	icons := map[string]string{
		"Head & Neck":          "M16 7a4 4 0 11-8 0 4 4 0 018 0zM12 14a7 7 0 00-7 7h14a7 7 0 00-7-7z",
		"Shoulders & Scapulae": "M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10",
		"Spine":                "M4 6h16M4 10h16M4 14h16M4 18h16",
		"Pelvis & Hips":        "M12 6V4m0 2a2 2 0 100 4m0-4a2 2 0 110 4m-6 8a2 2 0 100-4m0 4a2 2 0 110-4m0 4v2m0-6V4m6 6v10m6-2a2 2 0 100-4m0 4a2 2 0 110-4m0 4v2m0-6V4",
		"Lower Extremities":    "M13.828 10.172a4 4 0 00-5.656 0l-4 4a4 4 0 105.656 5.656l1.102-1.101m-.758-4.899a4 4 0 005.656 0l4-4a4 4 0 00-5.656-5.656l-1.1 1.1",
		"Ankles & Feet":        "M3.055 11H5a2 2 0 012 2v1a2 2 0 002 2 2 2 0 012 2v2.945M8 3.935V5.5A2.5 2.5 0 0010.5 8h.5a2 2 0 012 2 2 2 0 104 0 2 2 0 012-2h1.064M15 20.488V18a2 2 0 012-2h3.064",
	}

	regions := []map[string]interface{}{
		{"Title": "Head & Neck", "Data": analysisData["head_neck"], "Icon": icons["Head & Neck"]},
		{"Title": "Shoulders & Scapulae", "Data": analysisData["shoulders"], "Icon": icons["Shoulders & Scapulae"]},
		{"Title": "Spine", "Data": analysisData["spine"], "Icon": icons["Spine"]},
		{"Title": "Pelvis & Hips", "Data": analysisData["pelvis"], "Icon": icons["Pelvis & Hips"]},
		{"Title": "Lower Extremities", "Data": analysisData["lower_extremities"], "Icon": icons["Lower Extremities"]},
		{"Title": "Ankles & Feet", "Data": analysisData["ankles_feet"], "Icon": icons["Ankles & Feet"]},
	}

	// Merge forward_head_posture into head_neck
	if headNeck, ok := analysisData["head_neck"].(map[string]interface{}); ok {
		if globalAlign, ok := analysisData["global_alignment"].(map[string]interface{}); ok {
			if fhp, ok := globalAlign["forward_head_posture"]; ok {
				headNeck["forward_head_posture"] = fhp
			}
		}
	}

	// Calculate score
	score := 100
	for _, region := range regions {
		dataMap, ok := region["Data"].(map[string]interface{})
		if !ok {
			continue
		}
		for _, v := range dataMap {
			if metric, ok := v.(map[string]interface{}); ok {
				sev, _ := metric["severity"].(string)
				switch sev {
				case "severe":
					score -= 15
				case "moderate":
					score -= 10
				case "mild":
					score -= 5
				}
			}
		}
	}
	if score < 0 {
		score = 0
	}

	analysisData["PostureScore"] = score
	analysisData["Regions"] = regions
}

func generateHTMLReport(outputDir string, data map[string]interface{}) {
	templatePath := os.Getenv("TEMPLATE_FILE")
	if templatePath == "" {
		templatePath = "template.html"
	}

	tmplFuncs := template.FuncMap{
		"dict": func(values ...interface{}) (map[string]interface{}, error) {
			if len(values)%2 != 0 {
				return nil, fmt.Errorf("invalid dict call")
			}
			dict := make(map[string]interface{}, len(values)/2)
			for i := 0; i < len(values); i += 2 {
				key, ok := values[i].(string)
				if !ok {
					return nil, fmt.Errorf("dict keys must be strings")
				}
				dict[key] = values[i+1]
			}
			return dict, nil
		},
		"mul": func(a, b float64) float64 {
			return a * b
		},
		"isMap": func(v interface{}) bool {
			return reflect.TypeOf(v).Kind() == reflect.Map
		},
		"formatKey": func(s string) string {
			return strings.ReplaceAll(strings.Title(strings.ReplaceAll(s, "_", " ")), " ", " ")
		},
		"seq": func(n int) []int {
			result := make([]int, n)
			for i := 0; i < n; i++ {
				result[i] = i
			}
			return result
		},
		"add": func(a, b int) int {
			return a + b
		},
		"toFloat": func(i int) float64 {
			return float64(i)
		},
	}

	tmpl, err := template.New(filepath.Base(templatePath)).Funcs(tmplFuncs).ParseFiles(templatePath)
	if err != nil {
		log.Printf("Warning: Could not parse HTML template: %v", err)
		return
	}

	reportPath := filepath.Join(outputDir, "report.html")
	reportFile, err := os.Create(reportPath)
	if err != nil {
		log.Printf("Warning: Could not create report.html: %v", err)
		return
	}
	defer reportFile.Close()

	if err := tmpl.Execute(reportFile, data); err != nil {
		log.Printf("Warning: Could not execute HTML template: %v", err)
	} else {
		fmt.Printf("HTML report saved to: %s\n", reportPath)
	}
}
