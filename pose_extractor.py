#!/usr/bin/env python3
"""
Deterministic pose extraction using MediaPipe.
Extracts landmarks and calculates posture metrics.
"""
import sys
import json
import math
import os
import cv2
import mediapipe as mp
from mediapipe.tasks import python
from mediapipe.tasks.python import vision

def extract_landmarks(image_path, view_name):
    """Extract pose landmarks from image."""
    image = cv2.imread(image_path)
    if image is None:
        return None
    
    # Convert to MediaPipe Image
    mp_image = mp.Image(image_format=mp.ImageFormat.SRGB, data=cv2.cvtColor(image, cv2.COLOR_BGR2RGB))
    
    # Create pose landmarker
    base_options = python.BaseOptions(model_asset_path='pose_landmarker_lite.task')
    options = vision.PoseLandmarkerOptions(
        base_options=base_options,
        output_segmentation_masks=False)
    
    with vision.PoseLandmarker.create_from_options(options) as landmarker:
        results = landmarker.detect(mp_image)
        
        if not results.pose_landmarks:
            return None
        
        h, w = image.shape[:2]
        landmarks = {}
        
        for idx, lm in enumerate(results.pose_landmarks[0]):
            landmarks[idx] = {
                'x': lm.x,
                'y': lm.y,
                'z': lm.z,
                'visibility': lm.visibility,
                'x_px': int(lm.x * w),
                'y_px': int(lm.y * h)
            }
        
        return landmarks

def calculate_angle(p1, p2, p3):
    """Calculate angle at p2 formed by p1-p2-p3."""
    a = math.sqrt((p2['x'] - p1['x'])**2 + (p2['y'] - p1['y'])**2)
    b = math.sqrt((p3['x'] - p2['x'])**2 + (p3['y'] - p2['y'])**2)
    c = math.sqrt((p3['x'] - p1['x'])**2 + (p3['y'] - p1['y'])**2)
    
    if a * b == 0:
        return None
    
    cos_angle = (a**2 + b**2 - c**2) / (2 * a * b)
    cos_angle = max(-1, min(1, cos_angle))
    return math.degrees(math.acos(cos_angle))

def calculate_metrics(landmarks_dict, user_height_cm=None):
    """Calculate clinical posture metrics from landmarks."""
    metrics = {}
    
    front = landmarks_dict.get('front')
    right = landmarks_dict.get('right')
    left = landmarks_dict.get('left')
    back = landmarks_dict.get('back')
    
    # Calibration: estimate pixel to mm ratio
    if front and user_height_cm:
        nose = front.get(0)
        ankle = front.get(28) or front.get(27)
        if nose and ankle:
            body_height_px = abs(nose['y_px'] - ankle['y_px'])
            px_to_mm = (user_height_cm * 10) / body_height_px if body_height_px > 0 else 1
        else:
            px_to_mm = 1
    else:
        px_to_mm = 1
    
    # 1. Craniovertebral Angle (from right lateral)
    if right:
        ear = right.get(8)  # right ear
        shoulder = right.get(12)  # right shoulder
        if ear and shoulder and ear['visibility'] > 0.5 and shoulder['visibility'] > 0.5:
            dx = ear['x'] - shoulder['x']
            dy = ear['y'] - shoulder['y']
            angle = math.degrees(math.atan2(dy, dx))
            cva = 90 - abs(angle)
            metrics['craniovertebral_angle'] = {
                'value': round(cva, 1),
                'unit': 'degrees',
                'confidence': min(ear['visibility'], shoulder['visibility'])
            }
    
    # 2. Forward Head Posture (from right lateral)
    if right:
        ear = right.get(8)
        shoulder = right.get(12)
        if ear and shoulder and ear['visibility'] > 0.5 and shoulder['visibility'] > 0.5:
            fhp_px = abs(ear['x_px'] - shoulder['x_px'])
            fhp_mm = fhp_px * px_to_mm
            metrics['forward_head_posture'] = {
                'value': round(fhp_mm, 1),
                'unit': 'mm',
                'confidence': min(ear['visibility'], shoulder['visibility'])
            }
    
    # 3. Shoulder Height Delta (from front)
    if front:
        left_shoulder = front.get(11)
        right_shoulder = front.get(12)
        if left_shoulder and right_shoulder and left_shoulder['visibility'] > 0.5 and right_shoulder['visibility'] > 0.5:
            delta_px = abs(left_shoulder['y_px'] - right_shoulder['y_px'])
            delta_mm = delta_px * px_to_mm
            metrics['shoulder_height_delta'] = {
                'value': round(delta_mm, 1),
                'unit': 'mm',
                'confidence': min(left_shoulder['visibility'], right_shoulder['visibility'])
            }
    
    # 4. Thoracic Kyphosis (from right lateral - simplified)
    if right:
        shoulder = right.get(12)
        hip = right.get(24)
        if shoulder and hip and shoulder['visibility'] > 0.5 and hip['visibility'] > 0.5:
            # Estimate mid-back point
            mid_x = (shoulder['x'] + hip['x']) / 2
            mid_y = (shoulder['y'] + hip['y']) / 2
            mid_back = {'x': mid_x, 'y': mid_y}
            
            angle = calculate_angle(shoulder, mid_back, hip)
            if angle:
                kyphosis = 180 - angle
                metrics['thoracic_kyphosis'] = {
                    'value': round(kyphosis, 1),
                    'unit': 'degrees',
                    'confidence': min(shoulder['visibility'], hip['visibility']) * 0.7
                }
    
    # 5. Knee Valgus (from front - left knee)
    if front:
        left_hip = front.get(23)
        left_knee = front.get(25)
        left_ankle = front.get(27)
        if all([left_hip, left_knee, left_ankle]) and all([x['visibility'] > 0.5 for x in [left_hip, left_knee, left_ankle]]):
            angle = calculate_angle(left_hip, left_knee, left_ankle)
            if angle:
                valgus = 180 - angle
                metrics['knee_valgus_varus'] = {
                    'value': round(valgus, 1),
                    'unit': 'degrees',
                    'confidence': min(left_hip['visibility'], left_knee['visibility'], left_ankle['visibility'])
                }
    
    # 6. Foot Progression Angle (from front)
    if front:
        heel = front.get(29)  # left heel
        toe = front.get(31)   # left foot index
        if heel and toe and heel['visibility'] > 0.5 and toe['visibility'] > 0.5:
            dx = toe['x'] - heel['x']
            dy = toe['y'] - heel['y']
            angle = math.degrees(math.atan2(dx, dy))
            metrics['foot_progression_angle'] = {
                'value': round(abs(angle), 1),
                'unit': 'degrees',
                'confidence': min(heel['visibility'], toe['visibility'])
            }
    
    return metrics

def main():
    if len(sys.argv) < 5:
        print(json.dumps({'error': 'Usage: pose_extractor.py <front> <left> <right> <back> [height_cm] [output_dir]'}))
        sys.exit(1)
    
    front_path = sys.argv[1]
    left_path = sys.argv[2]
    right_path = sys.argv[3]
    back_path = sys.argv[4]
    
    # Parse optional arguments
    height_cm = None
    output_dir = None
    
    if len(sys.argv) > 5:
        # Check if arg 5 is a number (height) or path (output_dir)
        try:
            height_cm = float(sys.argv[5])
            if len(sys.argv) > 6:
                output_dir = sys.argv[6]
        except ValueError:
            # It's a path, not a number
            output_dir = sys.argv[5]
    
    # Extract landmarks from all views
    landmarks_dict = {
        'front': extract_landmarks(front_path, 'front'),
        'left': extract_landmarks(left_path, 'left'),
        'right': extract_landmarks(right_path, 'right'),
        'back': extract_landmarks(back_path, 'back')
    }
    
    # Calculate metrics
    metrics = calculate_metrics(landmarks_dict, height_cm)
    
    # Draw landmarks on images if output_dir provided
    if output_dir:
        os.makedirs(output_dir, exist_ok=True)
        draw_landmarks_on_image(front_path, landmarks_dict['front'], metrics, 
                                os.path.join(output_dir, 'front_annotated.jpg'), 'front')
        draw_landmarks_on_image(left_path, landmarks_dict['left'], metrics, 
                                os.path.join(output_dir, 'left_annotated.jpg'), 'left')
        draw_landmarks_on_image(right_path, landmarks_dict['right'], metrics, 
                                os.path.join(output_dir, 'right_annotated.jpg'), 'right')
        draw_landmarks_on_image(back_path, landmarks_dict['back'], metrics, 
                                os.path.join(output_dir, 'back_annotated.jpg'), 'back')
    
    # Output JSON
    output = {
        'landmarks': landmarks_dict,
        'metrics': metrics,
        'calibration': {
            'user_height_cm': height_cm,
            'method': 'user_provided' if height_cm else 'uncalibrated'
        }
    }
    
    print(json.dumps(output, indent=2))

def draw_landmarks_on_image(image_path, landmarks, metrics, output_path, view_name):
    """Draw landmarks and measurements on image."""
    if landmarks is None:
        return
    
    image = cv2.imread(image_path)
    if image is None:
        return
    
    h, w = image.shape[:2]
    
    # Draw all landmarks as small circles
    for idx, lm in landmarks.items():
        if lm['visibility'] > 0.5:
            cv2.circle(image, (lm['x_px'], lm['y_px']), 3, (0, 255, 0), -1)
    
    # Draw measurements based on view
    if view_name == 'right' or view_name == 'left':
        # Craniovertebral angle
        ear = landmarks.get(8) if view_name == 'right' else landmarks.get(7)
        shoulder = landmarks.get(12) if view_name == 'right' else landmarks.get(11)
        if ear and shoulder and ear['visibility'] > 0.5 and shoulder['visibility'] > 0.5:
            cv2.line(image, (ear['x_px'], ear['y_px']), 
                    (shoulder['x_px'], shoulder['y_px']), (255, 0, 0), 2)
            if 'craniovertebral_angle' in metrics:
                cv2.putText(image, f"CVA: {metrics['craniovertebral_angle']['value']:.1f}deg", 
                           (shoulder['x_px'] + 10, shoulder['y_px'] - 10),
                           cv2.FONT_HERSHEY_SIMPLEX, 0.5, (255, 0, 0), 2)
        
        # Forward head posture
        if ear and shoulder and 'forward_head_posture' in metrics:
            cv2.line(image, (ear['x_px'], ear['y_px']), 
                    (ear['x_px'], shoulder['y_px']), (0, 0, 255), 2)
            cv2.putText(image, f"FHP: {metrics['forward_head_posture']['value']:.1f}mm", 
                       (ear['x_px'] + 10, ear['y_px'] + 20),
                       cv2.FONT_HERSHEY_SIMPLEX, 0.5, (0, 0, 255), 2)
        
        # Thoracic kyphosis (shoulder to hip)
        hip = landmarks.get(24) if view_name == 'right' else landmarks.get(23)
        if shoulder and hip and shoulder['visibility'] > 0.5 and hip['visibility'] > 0.5:
            mid_x = (shoulder['x_px'] + hip['x_px']) // 2
            mid_y = (shoulder['y_px'] + hip['y_px']) // 2
            cv2.line(image, (shoulder['x_px'], shoulder['y_px']), 
                    (mid_x, mid_y), (255, 165, 0), 2)
            cv2.line(image, (mid_x, mid_y), (hip['x_px'], hip['y_px']), (255, 165, 0), 2)
            if 'thoracic_kyphosis' in metrics:
                cv2.putText(image, f"T-Kyphosis: {metrics['thoracic_kyphosis']['value']:.1f}deg", 
                           (mid_x + 10, mid_y),
                           cv2.FONT_HERSHEY_SIMPLEX, 0.5, (255, 165, 0), 2)
    
    elif view_name == 'front':
        # Shoulder height delta
        left_shoulder = landmarks.get(11)
        right_shoulder = landmarks.get(12)
        if left_shoulder and right_shoulder:
            cv2.line(image, (left_shoulder['x_px'], left_shoulder['y_px']),
                    (right_shoulder['x_px'], right_shoulder['y_px']), (255, 0, 0), 2)
            if 'shoulder_height_delta' in metrics:
                mid_x = (left_shoulder['x_px'] + right_shoulder['x_px']) // 2
                mid_y = (left_shoulder['y_px'] + right_shoulder['y_px']) // 2
                cv2.putText(image, f"Shoulder Delta: {metrics['shoulder_height_delta']['value']:.1f}mm", 
                           (mid_x - 80, mid_y - 10),
                           cv2.FONT_HERSHEY_SIMPLEX, 0.5, (255, 0, 0), 2)
        
        # Knee valgus/varus (left leg)
        left_hip = landmarks.get(23)
        left_knee = landmarks.get(25)
        left_ankle = landmarks.get(27)
        if all([left_hip, left_knee, left_ankle]):
            cv2.line(image, (left_hip['x_px'], left_hip['y_px']),
                    (left_knee['x_px'], left_knee['y_px']), (0, 255, 255), 2)
            cv2.line(image, (left_knee['x_px'], left_knee['y_px']),
                    (left_ankle['x_px'], left_ankle['y_px']), (0, 255, 255), 2)
            if 'knee_valgus_varus' in metrics:
                cv2.putText(image, f"Knee: {metrics['knee_valgus_varus']['value']:.1f}deg", 
                           (left_knee['x_px'] + 10, left_knee['y_px']),
                           cv2.FONT_HERSHEY_SIMPLEX, 0.5, (0, 255, 255), 2)
        
        # Knee valgus/varus (right leg)
        right_hip = landmarks.get(24)
        right_knee = landmarks.get(26)
        right_ankle = landmarks.get(28)
        if all([right_hip, right_knee, right_ankle]):
            cv2.line(image, (right_hip['x_px'], right_hip['y_px']),
                    (right_knee['x_px'], right_knee['y_px']), (0, 255, 255), 2)
            cv2.line(image, (right_knee['x_px'], right_knee['y_px']),
                    (right_ankle['x_px'], right_ankle['y_px']), (0, 255, 255), 2)
        
        # Foot progression angle (left foot)
        left_heel = landmarks.get(29)
        left_toe = landmarks.get(31)
        if left_heel and left_toe and left_heel['visibility'] > 0.5:
            cv2.line(image, (left_heel['x_px'], left_heel['y_px']),
                    (left_toe['x_px'], left_toe['y_px']), (255, 0, 255), 2)
            if 'foot_progression_angle' in metrics:
                cv2.putText(image, f"Foot: {metrics['foot_progression_angle']['value']:.1f}deg", 
                           (left_heel['x_px'] - 50, left_heel['y_px'] + 20),
                           cv2.FONT_HERSHEY_SIMPLEX, 0.5, (255, 0, 255), 2)
        
        # Foot progression angle (right foot)
        right_heel = landmarks.get(30)
        right_toe = landmarks.get(32)
        if right_heel and right_toe and right_heel['visibility'] > 0.5:
            cv2.line(image, (right_heel['x_px'], right_heel['y_px']),
                    (right_toe['x_px'], right_toe['y_px']), (255, 0, 255), 2)
    
    # Save annotated image
    cv2.imwrite(output_path, image)

if __name__ == '__main__':
    main()
