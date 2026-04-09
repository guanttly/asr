from PIL import Image, ImageDraw, ImageFont
import os

# Create a 512x512 image
size = 512
img = Image.new('RGB', (size, size), color=(15, 118, 110))

draw = ImageDraw.Draw(img)

try:
    font = ImageFont.truetype("/usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf", 200)
except IOError:
    try:
        font = ImageFont.truetype("arial", 200)
    except IOError:
        font = ImageFont.load_default()

text = "ASR"
bb = draw.textbbox((0,0), text, font=font)
w, h = bb[2] - bb[0], bb[3] - bb[1]

x = (size - w) / 2
y = (size - h) / 2 - 20

draw.text((x, y), text, fill=(255, 255, 255), font=font)

path = "frontend/public/logo.png"
os.makedirs(os.path.dirname(path), exist_ok=True)
img.save(path)
print("Logo generated successfully at", path)
