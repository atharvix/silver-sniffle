#!/usr/bin/env python3
"""Regenerate every app icon from one source image.

Source: frontend/public/kinjo-logo-dark.png (white Kinjo mark on black).

Having a single source is the point: the launcher icon, the Android
notification icon and the iOS app icon had drifted apart — Android was still
shipping the default Capacitor template icon (teal background, Ionic logo), and
the notification icon was a full-colour bitmap that Android renders as a white
square in the status bar.

The mark is inset to GLYPH_RATIO of the canvas. That is roughly Android's 66dp
safe zone inside the 108dp adaptive-icon grid, so no launcher mask can clip it.

Run:  python3 frontend/scripts/generate-app-icons.py
"""

from pathlib import Path

from PIL import Image

ROOT = Path(__file__).resolve().parents[1]
SOURCE = ROOT / "public" / "kinjo-logo-dark.png"

BACKGROUND = (0, 0, 0, 255)
# Inset so the mark never touches the canvas edge. Legacy icons are not masked,
# adaptive ones are (circle/squircle), so one conservative value serves both.
GLYPH_RATIO = 0.60
# Notification small icons get padded by the system already; a little more
# breathing room than full bleed keeps the glyph off the status-bar edges.
NOTIFICATION_RATIO = 0.80

ANDROID_RES = ROOT / "android" / "app" / "src" / "main" / "res"
# density -> px, per Android's icon size table
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


def build_mark() -> Image.Image:
    """The white mark on transparency, cropped to its own ink."""
    source = Image.open(SOURCE).convert("RGB")
    # Black background -> 0, white mark -> 255, so luminance doubles as alpha.
    alpha = source.convert("L")
    mark = Image.new("RGBA", source.size, (255, 255, 255, 0))
    mark.putalpha(alpha)

    ink = alpha.point(lambda value: 255 if value > 8 else 0).getbbox()
    if ink is None:
        raise SystemExit(f"{SOURCE} is blank — nothing to generate")
    return mark.crop(ink)


def compose(mark: Image.Image, size: int, ratio: float, background=None) -> Image.Image:
    """Centre the mark on a `size` square canvas, fitted into `ratio` of it."""
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


def write(path: Path, image: Image.Image) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    image.save(path)


def main() -> None:
    mark = build_mark()
    print(f"mark: {mark.width}x{mark.height} (from {SOURCE.name})")

    for density, size in LAUNCHER_SIZES.items():
        directory = ANDROID_RES / f"mipmap-{density}"
        icon = compose(mark, size, GLYPH_RATIO, BACKGROUND).convert("RGB")
        write(directory / "ic_launcher.png", icon)
        write(directory / "ic_launcher_round.png", icon)

    for density, size in ADAPTIVE_SIZES.items():
        write(
            ANDROID_RES / f"mipmap-{density}" / "ic_launcher_foreground.png",
            compose(mark, size, GLYPH_RATIO),
        )

    for density, size in NOTIFICATION_SIZES.items():
        write(
            ANDROID_RES / f"drawable-{density}" / "ic_notification.png",
            compose(mark, size, NOTIFICATION_RATIO),
        )

    # iOS masks the icon itself and forbids an alpha channel.
    write(IOS_ICON, compose(mark, 1024, GLYPH_RATIO, BACKGROUND).convert("RGB"))

    print("wrote android launcher, adaptive, notification and iOS icons")


if __name__ == "__main__":
    main()
