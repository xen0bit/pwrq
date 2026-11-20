"""Writes pkg/d1/testdata/vision_fixtures.json (and the pictures it reads): the processor's
pixel_values for pictures that reach the picture cap (media.cap_pixels, PIL), strong
up- and downscaling (torchvision), extreme aspect ratios, and alpha.

    python internal/tools/d1fixtures/vision_fixtures.py D1_2B_SNAPSHOT OUT_DIR
"""
import importlib
import json
import os
import sys

import numpy as np
import torch
from PIL import Image, ImageDraw
from transformers import AutoModel


def pictures(out):
    made = {}
    w, h = 1800, 1100  # 1.98 MP: past d1-2B's 1,638,400-pixel cap
    x = np.linspace(0, 255, w)[None, :, None]
    y = np.linspace(0, 255, h)[:, None, None]
    arr = np.concatenate([np.broadcast_to(x, (h, w, 1)), np.broadcast_to(y, (h, w, 1)),
                          np.broadcast_to((x + y) / 2, (h, w, 1))], axis=2).astype(np.uint8)
    img = Image.fromarray(arr)
    d = ImageDraw.Draw(img)
    for i in range(12):
        d.ellipse([60 + i * 140, 200 + (i % 3) * 200, 160 + i * 140, 300 + (i % 3) * 200], fill=(255 - i * 20, i * 20, 128))
    made["big.png"] = img
    img = Image.new("RGB", (40, 900), (10, 200, 30))
    ImageDraw.Draw(img).line([0, 0, 40, 900], fill=(255, 255, 255), width=3)
    made["tall.png"] = img
    img = Image.new("RGB", (9, 13), (200, 10, 10))
    img.putpixel((4, 6), (0, 0, 255))
    made["tiny.png"] = img
    img = Image.new("RGBA", (64, 64), (0, 0, 255, 0))
    ImageDraw.Draw(img).rectangle([16, 16, 48, 48], fill=(255, 128, 0, 128))
    made["alpha.png"] = img
    for name, im in made.items():
        im.save(os.path.join(out, name))
    return list(made)


def main(snap, out):
    names = pictures(out)
    model = AutoModel.from_pretrained(snap, trust_remote_code=True, dtype=torch.float32)
    eng = model.engine
    media = importlib.import_module(type(eng).__module__.rsplit(".", 1)[0] + ".media")
    cases = []
    for name in names:
        im = Image.open(os.path.join(out, name)).convert("RGB")
        with torch.inference_mode():
            text, parts = eng.media.encode([media.as_media(im)])
        px = parts[0]["pixel_values"].float().numpy().astype(np.float64).reshape(-1)
        cases.append({"name": name, "grid": parts[0]["image_grid_thw"][0].tolist(),
                      "tokens": text.count("<|image|>"), "sum": float(px.sum()), "sumsq": float((px * px).sum()),
                      "head": [float(v) for v in px[:256]], "tail": [float(v) for v in px[-256:]]})
    with open(os.path.join(out, "vision_fixtures.json"), "w") as f:
        json.dump({"cases": cases}, f)


if __name__ == "__main__":
    main(*sys.argv[1:])
