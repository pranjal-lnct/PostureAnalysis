package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/google/generative-ai-go/genai"
	openai "github.com/sashabaranov/go-openai"
	"google.golang.org/api/option"
)

func runGemini(ctx context.Context, promptText, frontPath, leftPath, rightPath, backPath string) string {
	apiKey := os.Getenv("GOOGLE_API_KEY")
	if apiKey == "" {
		apiKey = os.Getenv("GEMINI_API_KEY")
	}
	if apiKey == "" {
		log.Fatal("GOOGLE_API_KEY not set")
	}

	modelName := os.Getenv("GEMINI_MODEL")
	if modelName == "" {
		modelName = "gemini-3-flash-preview"
	}

	client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		log.Fatalf("Gemini client error: %v", err)
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

	resp, err := model.GenerateContent(ctx, parts...)
	if err != nil {
		log.Fatalf("Gemini error: %v", err)
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
		log.Fatal("OPENAI_API_KEY not set")
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

	resp, err := client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model:       modelName,
		Messages:    []openai.ChatCompletionMessage{{Role: openai.ChatMessageRoleUser, MultiContent: content}},
		Temperature: 0,
		ResponseFormat: &openai.ChatCompletionResponseFormat{
			Type: openai.ChatCompletionResponseFormatTypeJSONObject,
		},
	})
	if err != nil {
		log.Fatalf("OpenAI error: %v", err)
	}

	return resp.Choices[0].Message.Content
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
			"clinical_reasoning": {Type: genai.TypeString},
			"head_neck":          regionSchema("craniovertebral_angle", "lateral_head_tilt", "head_rotation"),
			"shoulders":          regionSchema("shoulder_height_delta", "shoulder_protraction", "scapular_winging"),
			"spine":              regionSchema("thoracic_kyphosis", "lumbar_lordosis", "lateral_deviation"),
			"pelvis":             regionSchema("pelvic_tilt", "pelvic_obliquity", "pelvic_rotation"),
			"lower_extremities":  regionSchema("knee_valgus_varus", "knee_hyperextension", "q_angle"),
			"ankles_feet":        regionSchema("foot_progression_angle", "ankle_pronation", "arch_height"),
			"global_alignment":   regionSchema("plumb_line_deviation", "forward_head_posture"),
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
					"compensation_chain":    {Type: genai.TypeString},
					"priority_areas":        {Type: genai.TypeArray, Items: &genai.Schema{Type: genai.TypeString}},
					"clinical_implications": {Type: genai.TypeString},
				},
				Required: []string{"muscle_imbalances", "compensation_chain", "priority_areas", "clinical_implications"},
			},
			"image_quality_notes": {Type: genai.TypeString},
			"overall_confidence":  {Type: genai.TypeNumber},
			"back_view_provided":  {Type: genai.TypeBoolean},
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

type Exercise struct {
	Name        string
	Description string
	Frequency   string
	Purpose     string
}

func generateExerciseRecommendations(analysisData map[string]interface{}) []Exercise {
	exercises := []Exercise{}

	if globalAlignment, ok := analysisData["global_alignment"].(map[string]interface{}); ok {
		if fhp, ok := globalAlignment["forward_head_posture"].(map[string]interface{}); ok {
			if severity, _ := fhp["severity"].(string); severity == "moderate" || severity == "severe" {
				exercises = append(exercises, Exercise{
					Name:        "Chin Tucks",
					Description: "Gently retract chin backward, hold for 5 seconds. Keep eyes level.",
					Frequency:   "3 sets of 10 reps, 2x daily",
					Purpose:     "Strengthens deep neck flexors and reduces forward head posture",
				})
			}
		}
	}

	if spine, ok := analysisData["spine"].(map[string]interface{}); ok {
		if kyphosis, ok := spine["thoracic_kyphosis"].(map[string]interface{}); ok {
			if severity, _ := kyphosis["severity"].(string); severity == "moderate" || severity == "severe" {
				exercises = append(exercises, Exercise{
					Name:        "Thoracic Extensions",
					Description: "Place hands behind head, extend upper back over foam roller. Hold 30 seconds.",
					Frequency:   "3-5 reps, 1-2x daily",
					Purpose:     "Improves thoracic spine mobility",
				})
			}
		}
	}

	if shoulders, ok := analysisData["shoulders"].(map[string]interface{}); ok {
		if protraction, ok := shoulders["shoulder_protraction"].(map[string]interface{}); ok {
			if severity, _ := protraction["severity"].(string); severity == "moderate" || severity == "severe" {
				exercises = append(exercises, Exercise{
					Name:        "Scapular Retractions",
					Description: "Squeeze shoulder blades together. Hold for 5 seconds.",
					Frequency:   "3 sets of 15 reps, 2x daily",
					Purpose:     "Strengthens rhomboids and middle trapezius",
				})
			}
		}
	}

	if len(exercises) >= 2 {
		exercises = append(exercises, Exercise{
			Name:        "Postural Awareness Practice",
			Description: "Stand against wall with heels, buttocks, shoulders, and head touching. Hold 30 seconds.",
			Frequency:   "2-3 times daily",
			Purpose:     "Develops kinesthetic awareness of optimal alignment",
		})
	}

	return exercises
}
