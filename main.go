package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"io"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/google/generative-ai-go/genai"
	"github.com/joho/godotenv"
	openai "github.com/sashabaranov/go-openai"
	"google.golang.org/api/option"
)

func main() {
	// Load .env
	if err := godotenv.Load(); err != nil {
		log.Println("Warning: No .env file found")
	}

	// Load provider config
	provider := os.Getenv("AI_PROVIDER")
	if provider == "" {
		provider = "gemini"
	}

	// Load config from env with defaults
	outputBase := os.Getenv("OUTPUT_DIR")
	if outputBase == "" {
		outputBase = "output"
	}
	promptFile := os.Getenv("PROMPT_FILE")
	if promptFile == "" {
		promptFile = "prompt.txt"
	}
	templateFile := os.Getenv("TEMPLATE_FILE")
	if templateFile == "" {
		templateFile = "template.html"
	}

	// Parse flags
	frontPtr := flag.String("front", "front.jpg", "Path to front view image")
	leftPtr := flag.String("left", "left.jpg", "Path to left view image")
	rightPtr := flag.String("right", "right.jpg", "Path to right view image")
	backPtr := flag.String("back", "back.jpg", "Path to back view image")
	modelPtr := flag.String("model", "", "Model to use (overrides env). Examples: gemini-3-flash-preview, gpt-4o-mini")
	providerPtr := flag.String("provider", "", "Provider: gemini or openai (overrides env)")
	flag.Parse()

	// Override provider/model from flags if provided
	if *providerPtr != "" {
		provider = *providerPtr
	}
	if *modelPtr != "" {
		os.Setenv("GEMINI_MODEL", *modelPtr)
		os.Setenv("OPENAI_MODEL", *modelPtr)
	}

	// Create Output Directory
	timestamp := time.Now().Format("2006-01-02_15-04-05")
	outputDir := filepath.Join(outputBase, timestamp)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		log.Fatalf("Error creating output directory: %v", err)
	}
	fmt.Printf("Created output directory: %s\n", outputDir)

	// Copy images
	copyFile := func(src, dstName string) {
		srcFile, err := os.Open(src)
		if err != nil {
			log.Printf("Warning: Could not open source image %s: %v", src, err)
			return
		}
		defer srcFile.Close()

		dstPath := filepath.Join(outputDir, dstName)
		dstFile, err := os.Create(dstPath)
		if err != nil {
			log.Printf("Warning: Could not create destination image %s: %v", dstPath, err)
			return
		}
		defer dstFile.Close()

		if _, err := io.Copy(dstFile, srcFile); err != nil {
			log.Printf("Warning: Could not copy image content: %v", err)
		}
	}

	// We copy the files to the output dir with standard names for easy reference, 
	// or we could keep original names. Standard names seems cleaner for the archive.
	copyFile(*frontPtr, "front"+filepath.Ext(*frontPtr))
	copyFile(*leftPtr, "left"+filepath.Ext(*leftPtr))
	copyFile(*rightPtr, "right"+filepath.Ext(*rightPtr))
	copyFile(*backPtr, "back"+filepath.Ext(*backPtr))

	// Read prompt from file
	promptBytes, err := os.ReadFile(promptFile)
	if err != nil {
		log.Fatalf("Error reading %s: %v", promptFile, err)
	}
	promptText := string(promptBytes)

	// Use a long timeout for multimodal processing
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	var resultText string

	fmt.Println("Starting Posture Analysis (Go)...")
	fmt.Printf("Using provider: %s\n", provider)

	if provider == "openai" {
		resultText = runOpenAI(ctx, promptText, *frontPtr, *leftPtr, *rightPtr, *backPtr)
	} else {
		resultText = runGemini(ctx, promptText, *frontPtr, *leftPtr, *rightPtr, *backPtr)
	}

	// Print to console
	fmt.Println("\n==================================================")
	fmt.Println("ANALYSIS RESULT")
	fmt.Println("==================================================\n")
	fmt.Println(resultText)
	fmt.Println("\n==================================================")

	// Save to file
	resultPath := filepath.Join(outputDir, "analysis.json")
	if err := os.WriteFile(resultPath, []byte(resultText), 0644); err != nil {
		log.Printf("Error saving analysis result: %v", err)
	} else {
		fmt.Printf("Analysis saved to: %s\n", resultPath)
	}
	// Parse JSON for report
	var analysisData map[string]interface{}
	// Clean the text in case model added markdown blocks despite prompt
	cleanJson := strings.TrimSpace(resultText)
	cleanJson = strings.TrimPrefix(cleanJson, "```json")
	cleanJson = strings.TrimPrefix(cleanJson, "```")
	cleanJson = strings.TrimSuffix(cleanJson, "```")
	
	if err := json.Unmarshal([]byte(cleanJson), &analysisData); err != nil {
		log.Printf("Warning: Could not parse JSON for HTML report: %v", err)
	} else {
        // Inject image paths relative to output folder
        imagesMap := map[string]string{
            "front": "front" + filepath.Ext(*frontPtr),
            "left": "left" + filepath.Ext(*leftPtr),
            "right": "right" + filepath.Ext(*rightPtr),
            "back": "back" + filepath.Ext(*backPtr),
        }
        analysisData["input_images"] = imagesMap

        // Helper string maps for icons
        icons := map[string]string{
            "Head & Neck":           "M16 7a4 4 0 11-8 0 4 4 0 018 0zM12 14a7 7 0 00-7 7h14a7 7 0 00-7-7z",
            "Shoulders & Scapulae":  "M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10",
            "Spine":                 "M4 6h16M4 10h16M4 14h16M4 18h16",
            "Pelvis & Hips":         "M12 6V4m0 2a2 2 0 100 4m0-4a2 2 0 110 4m-6 8a2 2 0 100-4m0 4a2 2 0 110-4m0 4v2m0-6V4m6 6v10m6-2a2 2 0 100-4m0 4a2 2 0 110-4m0 4v2m0-6V4",
            "Lower Extremities":     "M13.828 10.172a4 4 0 00-5.656 0l-4 4a4 4 0 105.656 5.656l1.102-1.101m-.758-4.899a4 4 0 005.656 0l4-4a4 4 0 00-5.656-5.656l-1.1 1.1",
            "Ankles & Feet":         "M3.055 11H5a2 2 0 012 2v1a2 2 0 002 2 2 2 0 012 2v2.945M8 3.935V5.5A2.5 2.5 0 0010.5 8h.5a2 2 0 012 2 2 2 0 104 0 2 2 0 012-2h1.064M15 20.488V18a2 2 0 012-2h3.064",
        }

        // Construct Regions list manually
        regions := []map[string]interface{}{
            {"Title": "Head & Neck", "Data": analysisData["head_neck"], "Icon": icons["Head & Neck"]},
            {"Title": "Shoulders & Scapulae", "Data": analysisData["shoulders"], "Icon": icons["Shoulders & Scapulae"]},
            {"Title": "Spine", "Data": analysisData["spine"], "Icon": icons["Spine"]},
            {"Title": "Pelvis & Hips", "Data": analysisData["pelvis"], "Icon": icons["Pelvis & Hips"]},
            {"Title": "Lower Extremities", "Data": analysisData["lower_extremities"], "Icon": icons["Lower Extremities"]},
            {"Title": "Ankles & Feet", "Data": analysisData["ankles_feet"], "Icon": icons["Ankles & Feet"]},
        }
        
        // Merge forward_head_posture into head_neck for display
        if headNeck, ok := analysisData["head_neck"].(map[string]interface{}); ok {
            if globalAlign, ok := analysisData["global_alignment"].(map[string]interface{}); ok {
                if fhp, ok := globalAlign["forward_head_posture"]; ok {
                    headNeck["forward_head_posture"] = fhp
                }
            }
        }
        // Calculate Posture Score (100 is perfect)
        score := 100
        for _, region := range regions {
            dataMap, ok := region["Data"].(map[string]interface{})
            if !ok { continue }
            
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
        if score < 0 { score = 0 }
        analysisData["PostureScore"] = score

        analysisData["Regions"] = regions

        // Generate exercise recommendations
        exercises := generateExerciseRecommendations(analysisData)
        analysisData["Exercises"] = exercises

		generateHTMLReport(outputDir, analysisData, templateFile)
	}
}

func runGemini(ctx context.Context, promptText, frontPath, leftPath, rightPath, backPath string) string {
	apiKey := os.Getenv("GOOGLE_API_KEY")
	if apiKey == "" {
		apiKey = os.Getenv("GEMINI_API_KEY")
	}
	if apiKey == "" {
		log.Fatal("Error: GOOGLE_API_KEY not set")
	}

	modelName := os.Getenv("GEMINI_MODEL")
	if modelName == "" {
		modelName = "gemini-3-flash-preview"
	}

	client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		log.Fatalf("Error creating Gemini client: %v", err)
	}
	defer client.Close()

	model := client.GenerativeModel(modelName)
	model.SetTemperature(0.0)
	model.SetTopK(1)
	model.ResponseMIMEType = "application/json"
	model.ResponseSchema = buildPostureSchema()

	var parts []genai.Part
	parts = append(parts, genai.Text(promptText))

	addImage := func(label, path string) {
		data, err := os.ReadFile(path)
		if err != nil {
			parts = append(parts, genai.Text(fmt.Sprintf("[%s] - Image not found", label)))
			return
		}
		mimeType := "jpeg"
		if strings.HasSuffix(strings.ToLower(path), ".png") {
			mimeType = "png"
		}
		parts = append(parts, genai.Text(fmt.Sprintf("[%s]", label)))
		parts = append(parts, genai.ImageData(mimeType, data))
	}

	addImage("Front View", frontPath)
	addImage("Left Side View", leftPath)
	addImage("Right Side View", rightPath)
	addImage("Back View", backPath)

	fmt.Printf("Sending request to Gemini (%s)...\n", modelName)
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

func runOpenAI(ctx context.Context, promptText, frontPath, leftPath, rightPath, backPath string) string {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		log.Fatal("Error: OPENAI_API_KEY not set")
	}

	modelName := os.Getenv("OPENAI_MODEL")
	if modelName == "" {
		modelName = "gpt-4o-mini"
	}

	client := openai.NewClient(apiKey)

	encodeImage := func(path string) (string, string) {
		data, err := os.ReadFile(path)
		if err != nil {
			return "", ""
		}
		mimeType := "image/jpeg"
		if strings.HasSuffix(strings.ToLower(path), ".png") {
			mimeType = "image/png"
		}
		return base64.StdEncoding.EncodeToString(data), mimeType
	}

	var content []openai.ChatMessagePart
	content = append(content, openai.ChatMessagePart{Type: openai.ChatMessagePartTypeText, Text: promptText})

	addImage := func(label, path string) {
		b64, mime := encodeImage(path)
		if b64 == "" {
			content = append(content, openai.ChatMessagePart{Type: openai.ChatMessagePartTypeText, Text: fmt.Sprintf("[%s] - Image not found", label)})
			return
		}
		content = append(content, openai.ChatMessagePart{Type: openai.ChatMessagePartTypeText, Text: fmt.Sprintf("[%s]", label)})
		content = append(content, openai.ChatMessagePart{
			Type: openai.ChatMessagePartTypeImageURL,
			ImageURL: &openai.ChatMessageImageURL{
				URL:    fmt.Sprintf("data:%s;base64,%s", mime, b64),
				Detail: openai.ImageURLDetailAuto,
			},
		})
	}

	addImage("Front View", frontPath)
	addImage("Left Side View", leftPath)
	addImage("Right Side View", rightPath)
	addImage("Back View", backPath)

	fmt.Printf("Sending request to OpenAI (%s)...\n", modelName)
	resp, err := client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: modelName,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleUser, MultiContent: content},
		},
		Temperature: 0,
		ResponseFormat: &openai.ChatCompletionResponseFormat{
			Type: openai.ChatCompletionResponseFormatTypeJSONObject,
		},
	})
	if err != nil {
		log.Fatalf("Error generating content: %v", err)
	}

	if len(resp.Choices) == 0 {
		log.Fatal("No response from OpenAI")
	}
	return resp.Choices[0].Message.Content
}

type Exercise struct {
	Name        string
	Description string
	Frequency   string
	Purpose     string
}

func buildPostureSchema() *genai.Schema {
	metricSchema := &genai.Schema{
		Type: genai.TypeObject,
		Properties: map[string]*genai.Schema{
			"value":      {Type: genai.TypeNumber, Nullable: true},
			"unit":       {Type: genai.TypeString},
			"severity":   {Type: genai.TypeString, Enum: []string{"normal", "mild", "moderate", "severe", "unknown"}},
			"confidence": {Type: genai.TypeNumber},
		},
		Required: []string{"value", "unit", "severity", "confidence"},
	}

	regionSchema := func(metrics ...string) *genai.Schema {
		props := map[string]*genai.Schema{"findings": {Type: genai.TypeString}}
		req := []string{"findings"}
		for _, m := range metrics {
			props[m] = metricSchema
			req = append(req, m)
		}
		return &genai.Schema{Type: genai.TypeObject, Properties: props, Required: req}
	}

	return &genai.Schema{
		Type: genai.TypeObject,
		Properties: map[string]*genai.Schema{
			"detected_views": {
				Type: genai.TypeObject,
				Properties: map[string]*genai.Schema{
					"front_detected": {Type: genai.TypeBoolean},
					"right_detected": {Type: genai.TypeBoolean},
					"left_detected":  {Type: genai.TypeBoolean},
					"back_detected":  {Type: genai.TypeBoolean},
				},
				Required: []string{"front_detected", "right_detected", "left_detected", "back_detected"},
			},
			"clinical_reasoning":   {Type: genai.TypeString},
			"head_neck":            regionSchema("craniovertebral_angle", "lateral_head_tilt", "head_rotation"),
			"shoulders":            regionSchema("shoulder_height_delta", "shoulder_protraction", "scapular_winging"),
			"spine":                regionSchema("thoracic_kyphosis", "lumbar_lordosis", "lateral_deviation"),
			"pelvis":               regionSchema("pelvic_tilt", "pelvic_obliquity", "pelvic_rotation"),
			"lower_extremities":    regionSchema("knee_valgus_varus", "knee_hyperextension", "q_angle"),
			"ankles_feet":          regionSchema("foot_progression_angle", "ankle_pronation", "arch_height"),
			"global_alignment":     regionSchema("plumb_line_deviation", "forward_head_posture"),
			"clinical_inference": {
				Type: genai.TypeObject,
				Properties: map[string]*genai.Schema{
					"muscle_imbalances": {
						Type: genai.TypeObject,
						Properties: map[string]*genai.Schema{
							"likely_tight": {Type: genai.TypeArray, Items: &genai.Schema{Type: genai.TypeString}},
							"likely_weak":  {Type: genai.TypeArray, Items: &genai.Schema{Type: genai.TypeString}},
						},
						Required: []string{"likely_tight", "likely_weak"},
					},
					"compensation_chain":     {Type: genai.TypeString},
					"priority_areas":         {Type: genai.TypeArray, Items: &genai.Schema{Type: genai.TypeString}},
					"clinical_implications":  {Type: genai.TypeString},
				},
				Required: []string{"muscle_imbalances", "compensation_chain", "priority_areas", "clinical_implications"},
			},
			"image_quality_notes":  {Type: genai.TypeString},
			"overall_confidence":   {Type: genai.TypeNumber},
			"back_view_provided":   {Type: genai.TypeBoolean},
			"annotations": {
				Type: genai.TypeArray,
				Items: &genai.Schema{
					Type: genai.TypeObject,
					Properties: map[string]*genai.Schema{
						"landmark":  {Type: genai.TypeString},
						"x_percent": {Type: genai.TypeNumber},
						"y_percent": {Type: genai.TypeNumber},
						"finding":   {Type: genai.TypeString},
						"severity":  {Type: genai.TypeString, Enum: []string{"moderate", "severe"}},
					},
					Required: []string{"landmark", "x_percent", "y_percent", "finding", "severity"},
				},
			},
		},
		Required: []string{"detected_views", "clinical_reasoning", "head_neck", "shoulders", "spine", "pelvis", "lower_extremities", "ankles_feet", "global_alignment", "clinical_inference", "image_quality_notes", "overall_confidence", "back_view_provided", "annotations"},
	}
}

func generateExerciseRecommendations(analysisData map[string]interface{}) []Exercise {
	exercises := []Exercise{}

	// Check for forward head posture
	if globalAlignment, ok := analysisData["global_alignment"].(map[string]interface{}); ok {
		if fhp, ok := globalAlignment["forward_head_posture"].(map[string]interface{}); ok {
			if severity, _ := fhp["severity"].(string); severity == "moderate" || severity == "severe" {
				exercises = append(exercises, Exercise{
					Name:        "Chin Tucks",
					Description: "Gently retract chin backward (like making a double chin), hold for 5 seconds. Keep eyes level.",
					Frequency:   "3 sets of 10 reps, 2x daily",
					Purpose:     "Strengthens deep neck flexors and reduces forward head posture",
				})
			}
		}
	}

	// Check for thoracic kyphosis
	if spine, ok := analysisData["spine"].(map[string]interface{}); ok {
		if kyphosis, ok := spine["thoracic_kyphosis"].(map[string]interface{}); ok {
			if severity, _ := kyphosis["severity"].(string); severity == "moderate" || severity == "severe" {
				exercises = append(exercises, Exercise{
					Name:        "Thoracic Extensions",
					Description: "Place hands behind head, gently extend upper back over a foam roller or rolled towel. Hold 30 seconds.",
					Frequency:   "3-5 repetitions, 1-2x daily",
					Purpose:     "Improves thoracic spine mobility and reduces excessive kyphosis",
				})
			}
		}
	}

	// Check for shoulder protraction
	if shoulders, ok := analysisData["shoulders"].(map[string]interface{}); ok {
		if protraction, ok := shoulders["shoulder_protraction"].(map[string]interface{}); ok {
			if severity, _ := protraction["severity"].(string); severity == "moderate" || severity == "severe" {
				exercises = append(exercises, Exercise{
					Name:        "Scapular Retractions",
					Description: "Squeeze shoulder blades together as if holding a pencil between them. Hold for 5 seconds.",
					Frequency:   "3 sets of 15 reps, 2x daily",
					Purpose:     "Strengthens rhomboids and middle trapezius to improve shoulder posture",
				})
			}
		}
	}

	// Check for lumbar lordosis
	if spine, ok := analysisData["spine"].(map[string]interface{}); ok {
		if lordosis, ok := spine["lumbar_lordosis"].(map[string]interface{}); ok {
			if severity, _ := lordosis["severity"].(string); severity == "moderate" || severity == "severe" {
				exercises = append(exercises, Exercise{
					Name:        "Pelvic Tilts",
					Description: "Lie on back with knees bent. Flatten lower back against floor by tilting pelvis. Hold 5 seconds.",
					Frequency:   "3 sets of 12 reps, 1-2x daily",
					Purpose:     "Activates core muscles and normalizes lumbar curve",
				})
			}
		}
	}

	// Check for knee hyperextension
	if lowerExt, ok := analysisData["lower_extremities"].(map[string]interface{}); ok {
		if hyperext, ok := lowerExt["knee_hyperextension"].(map[string]interface{}); ok {
			if severity, _ := hyperext["severity"].(string); severity == "mild" || severity == "moderate" {
				exercises = append(exercises, Exercise{
					Name:        "Quadriceps Strengthening",
					Description: "Seated leg extensions with slight knee bend. Focus on controlled movement without locking knees.",
					Frequency:   "3 sets of 10 reps, 3x weekly",
					Purpose:     "Improves knee control and reduces hyperextension tendency",
				})
			}
		}
	}

	// Add general postural awareness exercise if multiple issues
	if len(exercises) >= 3 {
		exercises = append(exercises, Exercise{
			Name:        "Postural Awareness Practice",
			Description: "Stand against wall with heels, buttocks, shoulders, and head touching. Hold 30 seconds while breathing normally.",
			Frequency:   "2-3 times daily",
			Purpose:     "Develops kinesthetic awareness of optimal alignment",
		})
	}

	return exercises
}

func generateHTMLReport(outputDir string, data map[string]interface{}, tmplPath string) {
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

	tmpl, err := template.New(filepath.Base(tmplPath)).Funcs(tmplFuncs).ParseFiles(tmplPath)
	if err != nil {
		log.Printf("Warning: Could not parse HTML template: %v", err)
		return
	}

	reportPath := filepath.Join(outputDir, "report.html")
	ctxFile, err := os.Create(reportPath)
	if err != nil {
		log.Printf("Warning: Could not create report.html: %v", err)
		return
	}
	defer ctxFile.Close()

	if err := tmpl.Execute(ctxFile, data); err != nil {
		log.Printf("Warning: Could not execute HTML template: %v", err)
	} else {
		fmt.Printf("Report saved to: %s\n", reportPath)
	}
}
