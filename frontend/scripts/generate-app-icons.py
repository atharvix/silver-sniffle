#!/usr/bin/env python3
"""Regenerate every app icon with beautiful padding from one source image.

Source: frontend/public/kinjo-logo-dark.png.

Adaptive icon specification:
- Android adaptive icon canvas is 108dp x 108dp.
- The visible circular/squircle mask is the inner 72dp x 72dp.
- A glyph ratio of 0.38 on the 108dp canvas produces a ~41dp mark inside the
  72dp visible circle, providing ~15.5dp of balanced padding on all sides so the
  mark is never crowded or clipped.
- Launcher background: #000000 (dark brand icon — black canvas, white mark).
- Launcher glyph: #FFFFFF (white mark centered with generous breathing room).
- Notification small icon: pure monochrome white (#FFFFFF) with alpha on transparent,
  per Android status bar guidelines.
- iOS app icon: solid black RGB background (no alpha) with white mark at 0.42 ratio.

Run:  python3 frontend/scripts/generate-app-icons.py
"""

from pathlib import Path
from PIL import Image, ImageDraw

ROOT = Path(__file__).resolve().parents[1]
SOURCE = ROOT / "public" / "kinjo-logo-dark.png"

# Adaptive icons (108dp canvas, 72dp visible mask):
# 0.38 of 108dp = 41dp mark inside 72dp circle -> 57% of circle diameter,
# leaving ~15.5dp of luxurious padding around the mark.
ADAPTIVE_GLYPH_RATIO = 0.38

# Legacy icons (48px - 192px):
# Mark is 52% of the canvas, leaving ample padding within the circle.
LEGACY_GLYPH_RATIO = 0.52

# Notification silhouette for status bar:
NOTIFICATION_RATIO = 0.72

# iOS 1024x1024 icon:
IOS_GLYPH_RATIO = 0.42

WHITE_COLOR = (255, 255, 255)   # #FFFFFF white brand mark
BLACK_COLOR = (0, 0, 0)         # #000000 dark icon canvas

ANDROID_RES = ROOT / "android" / "app" / "src" / "main" / "res"
LAUNCHER_SIZES = {"mdpi": 48, "hdpi": 72, "xhdpi": 96, "xxhdpi": 144, "xxxhdpi": 192}
ADAPTIVE_SIZES = {"mdpi": 108, "hdpi": 162, "xhdpi": 216, "xxhdpi": 324, "xxxhdpi": 432}
NOTIFICATION_SIZES = {"mdpi": 24, "hdpi": 36, "xhdpi": 48, "xxhdpi": 72, "xxxhdpi": 96}

IOS_ICON = (
    ROOT
    / "ios"
    / "App"
    / "App"
    / "Assets.xcassets"
    / "AppIcon.appiconset"
    / "AppIcon-512@2x.png"
)


def build_mark(color: tuple[int, int, int]) -> Image.Image:
    """Extract the mark on transparency, tinted with `color` and cropped to ink."""
    source = Image.open(SOURCE).convert("RGB")
    alpha = source.convert("L")
    tinted = Image.new("RGBA", source.size, (*color, 0))
    tinted.putalpha(alpha)

    ink = alpha.point(lambda value: 255 if value > 8 else 0).getbbox()
    if ink is None:
        raise SystemExit(f"{SOURCE} is blank — nothing to generate")
    return tinted.crop(ink)


def compose(mark: Image.Image, size: int, ratio: float, background=None) -> Image.Image:
    """Center the mark on a `size` square canvas, fitted into `ratio` of it."""
    canvas = Image.new("RGBA", (size, size), background or (0, 0, 0, 0))
    inner = max(1, round(size * ratio))
    width, height = mark.size
    scale = min(inner / width, inner / height)
    resized = mark.resize(
        (max(1, round(width * scale)), max(1, round(height * scale))),
        Image.LANCZOS,
    )
    canvas.alpha_composite(
        resized, ((size - resized.width) // 2, (size - resized.height) // 2)
    )
    return canvas


def create_legacy_round_icon(mark: Image.Image, size: int, ratio: float) -> Image.Image:
    """Create an anti-aliased black circle icon with the white mark centered."""
    ss = 4  # 4x supersampling for ultra smooth circle edges
    canvas = Image.new("RGBA", (size * ss, size * ss), (0, 0, 0, 0))
    draw = ImageDraw.Draw(canvas)
    margin = 2 * ss
    draw.ellipse((margin, margin, size * ss - margin, size * ss - margin), fill=(*BLACK_COLOR, 255))
    canvas = canvas.resize((size, size), Image.LANCZOS)

    inner = max(1, round(size * ratio))
    w, h = mark.size
    scale = min(inner / w, inner / h)
    resized = mark.resize(
        (max(1, round(w * scale)), max(1, round(h * scale))),
        Image.LANCZOS,
    )
    canvas.alpha_composite(
        resized, ((size - resized.width) // 2, (size - resized.height) // 2)
    )
    return canvas


def create_legacy_square_icon(mark: Image.Image, size: int, ratio: float) -> Image.Image:
    """Create an anti-aliased black rounded rectangle icon with the white mark centered."""
    ss = 4
    canvas = Image.new("RGBA", (size * ss, size * ss), (0, 0, 0, 0))
    draw = ImageDraw.Draw(canvas)
    margin = 2 * ss
    corner_radius = round(size * ss * 0.20)
    draw.rounded_rectangle(
        (margin, margin, size * ss - margin, size * ss - margin),
        radius=corner_radius,
        fill=(*BLACK_COLOR, 255),
    )
    canvas = canvas.resize((size, size), Image.LANCZOS)

    inner = max(1, round(size * ratio))
    w, h = mark.size
    scale = min(inner / w, inner / h)
    resized = mark.resize(
        (max(1, round(w * scale)), max(1, round(h * scale))),
        Image.LANCZOS,
    )
    canvas.alpha_composite(
        resized, ((size - resized.width) // 2, (size - resized.height) // 2)
    )
    return canvas


def write(path: Path, image: Image.Image) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    image.save(path)


def main() -> None:
    white_mark = build_mark(WHITE_COLOR)
    print(f"mark: {white_mark.width}x{white_mark.height} (from {SOURCE.name})")

    # 1. Android Adaptive foreground icons (transparent canvas, white mark with padding)
    for density, size in ADAPTIVE_SIZES.items():
        write(
            ANDROID_RES / f"mipmap-{density}" / "ic_launcher_foreground.png",
            compose(white_mark, size, ADAPTIVE_GLYPH_RATIO),
        )

    # 2. Android Legacy launcher icons (black background with generous padding)
    for density, size in LAUNCHER_SIZES.items():
        directory = ANDROID_RES / f"mipmap-{density}"
        write(directory / "ic_launcher_round.png", create_legacy_round_icon(white_mark, size, LEGACY_GLYPH_RATIO))
        write(directory / "ic_launcher.png", create_legacy_square_icon(white_mark, size, LEGACY_GLYPH_RATIO))

    # 3. Android Notification silhouette (white mark on transparent)
    for density, size in NOTIFICATION_SIZES.items():
        write(
            ANDROID_RES / f"drawable-{density}" / "ic_notification.png",
            compose(white_mark, size, NOTIFICATION_RATIO),
        )

    # 4. iOS App Icon (solid black RGB canvas with white mark)
    ios_canvas = compose(white_mark, 1024, IOS_GLYPH_RATIO, (*BLACK_COLOR, 255)).convert("RGB")
    write(IOS_ICON, ios_canvas)

    print("Successfully generated Android launcher, adaptive, notification, and iOS icons with beautiful padding.")


if __name__ == "__main__":
    main()
