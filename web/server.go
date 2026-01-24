package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

//go:embed templates/*
var templates embed.FS

var uploadTmpl *template.Template

func init() {
	var err error
	uploadTmpl, err = template.ParseFS(templates, "templates/upload.html")
	if err != nil {
		log.Fatal(err)
	}
}

func main() {
	if err := godotenv.Load("../.env"); err != nil {
		godotenv.Load(".env")
	}

	http.HandleFunc("/", handleUpload)
	http.HandleFunc("/analyze", handleAnalyze)
	http.HandleFunc("/report/", handleReport)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	fmt.Printf("Server starting at http://localhost:%s\n", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func handleUpload(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	uploadTmpl.Execute(w, nil)
}

func handleAnalyze(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	r.ParseMultipartForm(50 << 20) // 50MB max

	// Create output directory
	timestamp := time.Now().Format("2006-01-02_15-04-05")
	outputBase := os.Getenv("OUTPUT_DIR")
	if outputBase == "" {
		outputBase = "../output"
	}
	outputDir := filepath.Join(outputBase, timestamp)
	os.MkdirAll(outputDir, 0755)

	// Save uploaded files
	views := []string{"front", "back", "left", "right"}
	paths := make(map[string]string)

	for _, view := range views {
		file, header, err := r.FormFile(view)
		if err != nil {
			http.Error(w, "Missing "+view+" image", http.StatusBadRequest)
			return
		}
		defer file.Close()

		ext := filepath.Ext(header.Filename)
		savePath := filepath.Join(outputDir, view+ext)
		dst, _ := os.Create(savePath)
		io.Copy(dst, file)
		dst.Close()
		paths[view] = savePath
	}

	// Run analysis
	promptFile := os.Getenv("PROMPT_FILE")
	if promptFile == "" {
		promptFile = "../prompt.txt"
	}
	promptBytes, _ := os.ReadFile(promptFile)
	promptText := string(promptBytes)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	provider := os.Getenv("AI_PROVIDER")
	if provider == "" {
		provider = "gemini"
	}

	var resultText string
	if provider == "openai" {
		resultText = runOpenAI(ctx, promptText, paths["front"], paths["left"], paths["right"], paths["back"])
	} else {
		resultText = runGemini(ctx, promptText, paths["front"], paths["left"], paths["right"], paths["back"])
	}

	// Save analysis
	os.WriteFile(filepath.Join(outputDir, "analysis.json"), []byte(resultText), 0644)

	// Generate report
	var analysisData map[string]interface{}
	cleanJson := strings.TrimSpace(resultText)
	cleanJson = strings.TrimPrefix(cleanJson, "```json")
	cleanJson = strings.TrimPrefix(cleanJson, "```")
	cleanJson = strings.TrimSuffix(cleanJson, "```")

	if err := json.Unmarshal([]byte(cleanJson), &analysisData); err != nil {
		http.Error(w, "Failed to parse analysis", http.StatusInternalServerError)
		return
	}

	// Inject image paths
	analysisData["input_images"] = map[string]string{
		"front": filepath.Base(paths["front"]),
		"left":  filepath.Base(paths["left"]),
		"right": filepath.Base(paths["right"]),
		"back":  filepath.Base(paths["back"]),
	}

	// Build regions and score
	buildRegionsAndScore(analysisData)

	// Generate exercises
	analysisData["Exercises"] = generateExerciseRecommendations(analysisData)

	// Generate HTML report
	templateFile := os.Getenv("TEMPLATE_FILE")
	if templateFile == "" {
		templateFile = "../template.html"
	}
	generateHTMLReport(outputDir, analysisData, templateFile)

	// Redirect to report
	http.Redirect(w, r, "/report/"+timestamp, http.StatusSeeOther)
}

func handleReport(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/report/")
	outputBase := os.Getenv("OUTPUT_DIR")
	if outputBase == "" {
		outputBase = "../output"
	}

	// Serve images from output dir
	if strings.Contains(r.URL.Path, ".png") || strings.Contains(r.URL.Path, ".jpg") {
		parts := strings.Split(r.URL.Path, "/")
		if len(parts) >= 4 {
			imgPath := filepath.Join(outputBase, parts[2], parts[3])
			http.ServeFile(w, r, imgPath)
			return
		}
	}

	reportPath := filepath.Join(outputBase, id, "report.html")
	http.ServeFile(w, r, reportPath)
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
		"mul": func(a, b float64) float64 { return a * b },
		"isMap": func(v interface{}) bool {
			return reflect.TypeOf(v) != nil && reflect.TypeOf(v).Kind() == reflect.Map
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
		"add": func(a, b int) int { return a + b },
		"toFloat": func(i int) float64 { return float64(i) },
	}

	tmpl, err := template.New(filepath.Base(tmplPath)).Funcs(tmplFuncs).ParseFiles(tmplPath)
	if err != nil {
		log.Printf("Template error: %v", err)
		return
	}

	reportPath := filepath.Join(outputDir, "report.html")
	f, _ := os.Create(reportPath)
	defer f.Close()
	tmpl.Execute(f, data)
}
