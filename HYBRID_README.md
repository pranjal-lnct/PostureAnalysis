# Hybrid Posture Analysis - Quick Start

## What's Different?

**Old Approach:** LLM analyzes images directly (variable results)  
**Hybrid Approach:** MediaPipe extracts landmarks → Math calculates angles → LLM interprets (deterministic)

## Installation

### 1. Python Dependencies
```bash
python3 -m venv venv
source venv/bin/activate
pip install mediapipe opencv-python numpy
```

### 2. Download MediaPipe Model
```bash
curl -sL -o pose_landmarker_lite.task \
  https://storage.googleapis.com/mediapipe-models/pose_landmarker/pose_landmarker_lite/float16/latest/pose_landmarker_lite.task
```

### 3. Build Go Application
```bash
go build -o posture_hybrid main_hybrid.go
```

## Usage

### Basic Analysis
```bash
./posture_hybrid \
  --front Photos/F2.jpg \
  --left Photos/S2.jpg \
  --right Photos/S2.jpg \
  --back Photos/B2.jpg
```

### With Height Calibration (Recommended)
```bash
./posture_hybrid \
  --front Photos/F2.jpg \
  --left Photos/S2.jpg \
  --right Photos/S2.jpg \
  --back Photos/B2.jpg \
  --height 175
```

### Using OpenAI Instead of Gemini
```bash
./posture_hybrid \
  --front Photos/F2.jpg \
  --left Photos/S2.jpg \
  --right Photos/S2.jpg \
  --back Photos/B2.jpg \
  --provider openai
```

## Output

Results saved to `output_hybrid/YYYY-MM-DD_HH-MM-SS/`:
- `metrics.json` - Measured values (deterministic)
- `interpretation.json` - LLM clinical analysis
- `analysis_hybrid.json` - Combined report

### Sample Output
```
=== MEASURED METRICS (Deterministic) ===
  Craniovertebral Angle: 20.8° (confidence: 1.00)
  Forward Head Posture: 52.5mm (confidence: 1.00)
  Shoulder Height Delta: 26.2mm (confidence: 1.00)
  Thoracic Kyphosis: 0.0° (confidence: 0.70)
  Knee Valgus/Varus: 0.6° (confidence: 0.99)
  Foot Progression Angle: 27.4° (confidence: 0.83)
```

## Testing Determinism

Run multiple times to verify identical measurements:
```bash
for i in {1..5}; do
  ./posture_hybrid --front Photos/F2.jpg --left Photos/S2.jpg \
    --right Photos/S2.jpg --back Photos/B2.jpg --height 175 \
    2>/dev/null | grep "Craniovertebral"
done
```

Expected: All 5 runs show **identical values**

## Python Script Standalone

You can also use the Python script directly:
```bash
source venv/bin/activate
python3 pose_extractor.py \
  Photos/F2.jpg Photos/S2.jpg Photos/S2.jpg Photos/B2.jpg 175 \
  2>/dev/null | python3 -m json.tool
```

## Files

- `pose_extractor.py` - MediaPipe landmark extraction + math
- `main_hybrid.go` - Go wrapper + LLM integration
- `prompt_hybrid.txt` - LLM prompt for interpretation
- `pose_landmarker_lite.task` - MediaPipe model (5.5MB)

## Advantages

✅ **100% deterministic measurements** (same images → same values)  
✅ **3x faster** (3-5s vs 10-15s)  
✅ **5x cheaper** (text-only LLM calls)  
✅ **Transparent** (see exact landmark coordinates)  
✅ **Auditable** (verify calculations)

## Limitations

⚠️ **Thoracic Kyphosis** - Currently unreliable (MediaPipe lacks T1-T12 landmarks)  
⚠️ **Pelvic Tilt** - Not measured (no ASIS/PSIS landmarks)  
⚠️ **Lumbar Lordosis** - Not measured (no L1-S1 landmarks)

Focus on metrics MediaPipe can measure accurately.

## Troubleshooting

### "Module 'cv2' not found"
```bash
source venv/bin/activate
pip install opencv-python
```

### "pose_landmarker_lite.task not found"
```bash
curl -sL -o pose_landmarker_lite.task \
  https://storage.googleapis.com/mediapipe-models/pose_landmarker/pose_landmarker_lite/float16/latest/pose_landmarker_lite.task
```

### Python script fails
Make sure you're in the venv:
```bash
source venv/bin/activate
which python3  # Should show ./venv/bin/python3
```

## Documentation

- `HYBRID_RESULTS.md` - Detailed comparison & results
- `VARIABILITY_ANALYSIS.md` - Why old approach varied
- `README.md` - Original project documentation
