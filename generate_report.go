package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"strings"
)

func main() {
	jsonPath := flag.String("json", "", "Path to analysis.json file")
	flag.Parse()

	if *jsonPath == "" {
		// Default to the path the user mentioned, or just fail
		fmt.Println("Usage: go run generate_report.go --json <path/to/analysis.json>")
		
		// For convenience given the user request, let's try the specific path if it exists
		defaultPath := "output/2026-01-03_21-58-26/analysis.json"
		if _, err := os.Stat(defaultPath); err == nil {
			fmt.Printf("No path provided, using: %s\n", defaultPath)
			*jsonPath = defaultPath
		} else {
			return
		}
	}

	// Read JSON
	dataBytes, err := os.ReadFile(*jsonPath)
	if err != nil {
		log.Fatalf("Error reading JSON file: %v", err)
	}

	var analysisData map[string]interface{}
	if err := json.Unmarshal(dataBytes, &analysisData); err != nil {
		log.Fatalf("Error parsing JSON: %v", err)
	}

	// Output dir should remain same as json dir
	outputDir := filepath.Dir(*jsonPath)
	
	// Inject images by looking for them
	imagesMap := make(map[string]string)
	findImage := func(prefix string) string {
		exts := []string{".jpg", ".jpeg", ".png", ".webp", ".JPG", ".PNG"}
		for _, ext := range exts {
			fname := prefix + ext
			path := filepath.Join(outputDir, fname)
			if _, err := os.Stat(path); err == nil {
				return fname
			}
		}
		return ""
	}
	
	if img := findImage("front"); img != "" { imagesMap["front"] = img }
	if img := findImage("left"); img != "" { imagesMap["left"] = img }
	if img := findImage("right"); img != "" { imagesMap["right"] = img }
	if img := findImage("back"); img != "" { imagesMap["back"] = img }
	
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
        // Calculate Posture Score (100 is perfect)
        score := 100
        for _, region := range regions {
            dataMap, ok := region["Data"].(map[string]interface{})
            if !ok { continue }
            
            // findings string doesn't count
            
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

        // Add to analysisData so template can use it
        analysisData["Regions"] = regions

        // Generate exercise recommendations (imported from main.go logic)
        exercises := generateExerciseRecommendationsLocal(analysisData)
        analysisData["Exercises"] = exercises

	generateHTMLReport(outputDir, analysisData)
}

func generateExerciseRecommendationsLocal(analysisData map[string]interface{}) []map[string]string {
	exercises := []map[string]string{}

	// Check for forward head posture
	if globalAlignment, ok := analysisData["global_alignment"].(map[string]interface{}); ok {
		if fhp, ok := globalAlignment["forward_head_posture"].(map[string]interface{}); ok {
			if severity, _ := fhp["severity"].(string); severity == "moderate" || severity == "severe" {
				exercises = append(exercises, map[string]string{
					"Name":        "Chin Tucks",
					"Description": "Gently retract chin backward (like making a double chin), hold for 5 seconds. Keep eyes level.",
					"Frequency":   "3 sets of 10 reps, 2x daily",
					"Purpose":     "Strengthens deep neck flexors and reduces forward head posture",
				})
			}
		}
	}

	// Check for thoracic kyphosis
	if spine, ok := analysisData["spine"].(map[string]interface{}); ok {
		if kyphosis, ok := spine["thoracic_kyphosis"].(map[string]interface{}); ok {
			if severity, _ := kyphosis["severity"].(string); severity == "moderate" || severity == "severe" {
				exercises = append(exercises, map[string]string{
					"Name":        "Thoracic Extensions",
					"Description": "Place hands behind head, gently extend upper back over a foam roller or rolled towel. Hold 30 seconds.",
					"Frequency":   "3-5 repetitions, 1-2x daily",
					"Purpose":     "Improves thoracic spine mobility and reduces excessive kyphosis",
				})
			}
		}
	}

	// Check for shoulder protraction
	if shoulders, ok := analysisData["shoulders"].(map[string]interface{}); ok {
		if protraction, ok := shoulders["shoulder_protraction"].(map[string]interface{}); ok {
			if severity, _ := protraction["severity"].(string); severity == "moderate" || severity == "severe" {
				exercises = append(exercises, map[string]string{
					"Name":        "Scapular Retractions",
					"Description": "Squeeze shoulder blades together as if holding a pencil between them. Hold for 5 seconds.",
					"Frequency":   "3 sets of 15 reps, 2x daily",
					"Purpose":     "Strengthens rhomboids and middle trapezius to improve shoulder posture",
				})
			}
		}
	}

	// Check for lumbar lordosis
	if spine, ok := analysisData["spine"].(map[string]interface{}); ok {
		if lordosis, ok := spine["lumbar_lordosis"].(map[string]interface{}); ok {
			if severity, _ := lordosis["severity"].(string); severity == "moderate" || severity == "severe" {
				exercises = append(exercises, map[string]string{
					"Name":        "Pelvic Tilts",
					"Description": "Lie on back with knees bent. Flatten lower back against floor by tilting pelvis. Hold 5 seconds.",
					"Frequency":   "3 sets of 12 reps, 1-2x daily",
					"Purpose":     "Activates core muscles and normalizes lumbar curve",
				})
			}
		}
	}

	// Check for knee hyperextension
	if lowerExt, ok := analysisData["lower_extremities"].(map[string]interface{}); ok {
		if hyperext, ok := lowerExt["knee_hyperextension"].(map[string]interface{}); ok {
			if severity, _ := hyperext["severity"].(string); severity == "mild" || severity == "moderate" {
				exercises = append(exercises, map[string]string{
					"Name":        "Quadriceps Strengthening",
					"Description": "Seated leg extensions with slight knee bend. Focus on controlled movement without locking knees.",
					"Frequency":   "3 sets of 10 reps, 3x weekly",
					"Purpose":     "Improves knee control and reduces hyperextension tendency",
				})
			}
		}
	}

	// Add general postural awareness exercise if multiple issues
	if len(exercises) >= 3 {
		exercises = append(exercises, map[string]string{
			"Name":        "Postural Awareness Practice",
			"Description": "Stand against wall with heels, buttocks, shoulders, and head touching. Hold 30 seconds while breathing normally.",
			"Frequency":   "2-3 times daily",
			"Purpose":     "Develops kinesthetic awareness of optimal alignment",
		})
	}

	return exercises
}

func generateHTMLReport(outputDir string, data map[string]interface{}) {
	// Look for template in current dir
	tmplPath := "template.html"

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

	tmpl, err := template.New("template.html").Funcs(tmplFuncs).ParseFiles(tmplPath)
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
