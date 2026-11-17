#!/usr/bin/env python3
"""WCAG 2.1 contrast audit for the two themes (CHG-0007 AC-06).

Parses the semantic tokens straight out of globals.css so the numbers can never
drift from the shipped stylesheet. Fails if any text pair is below AA (4.5:1).

Why a script instead of a claim: low-contrast micro text is invisible in review
screenshots but painful in daily use, and this is exactly the kind of value that
gets hand-waved. Numbers, or it did not happen.
"""

from __future__ import annotations

import re
import sys
from datetime import datetime, timezone
from pathlib import Path

CSS = Path(__file__).resolve().parent.parent / "frontend" / "app" / "globals.css"
AA = 4.5


def srgb_to_linear(c: float) -> float:
    return c / 12.92 if c <= 0.04045 else ((c + 0.055) / 1.055) ** 2.4


def luminance(hex_color: str) -> float:
    h = hex_color.lstrip("#")
    r, g, b = (int(h[i : i + 2], 16) / 255 for i in (0, 2, 4))
    return 0.2126 * srgb_to_linear(r) + 0.7152 * srgb_to_linear(g) + 0.0722 * srgb_to_linear(b)


def ratio(fg: str, bg: str) -> float:
    a, b = luminance(fg), luminance(bg)
    lighter, darker = max(a, b), min(a, b)
    return (lighter + 0.05) / (darker + 0.05)


def theme_block(css: str, selector: str) -> dict[str, str]:
    """Collect --token: value pairs inside one selector block."""
    start = css.find(selector)
    if start == -1:
        raise SystemExit(f"selector not found: {selector}")
    end = css.find("}", start)
    block = css[start:end]
    return {m.group(1): m.group(2) for m in re.finditer(r"(--[a-z0-9-]+):\s*(#[0-9a-fA-F]{6})", block)}


def main() -> int:
    css = CSS.read_text(encoding="utf-8")
    out: list[str] = []

    def emit(line: str) -> None:
        print(line)
        out.append(line)

    themes = {
        "light": theme_block(css, ':root,\n:root[data-theme="light"]'),
        "dark": theme_block(css, ':root[data-theme="dark"]'),
    }

    # Pairs that are actually rendered as text. Decorative uses (2px bars, dots,
    # glow gradients) are intentionally excluded: WCAG applies to text.
    pairs = [
        ("text-1", "--text-1", "--bg", "body text"),
        ("text-2", "--text-2", "--bg", "secondary text"),
        ("text-3", "--text-3", "--bg", "micro / metadata text"),
        ("lamp", "--lamp", "--bg", "accent text (.tag--manual)"),
        ("text-1-on-surface", "--text-1", "--surface", "text on panel"),
        ("text-3-on-surface", "--text-3", "--surface", "metadata on panel"),
    ]

    failures: list[str] = []
    emit(f"{'theme':6} {'pair':22} {'fg':9} {'bg':9} {'ratio':>6}  verdict")
    emit("-" * 72)

    for theme_name, tokens in themes.items():
        for label, fg_key, bg_key, _desc in pairs:
            fg, bg = tokens.get(fg_key), tokens.get(bg_key)
            if not fg or not bg:
                failures.append(f"{theme_name}: missing token {fg_key} or {bg_key}")
                continue
            r = ratio(fg, bg)
            verdict = "PASS" if r >= AA else "FAIL"
            if r < AA:
                failures.append(f"{theme_name}/{label}: {r:.2f}:1 < {AA}:1 ({fg} on {bg})")
            emit(f"{theme_name:6} {label:22} {fg:9} {bg:9} {r:6.2f}  {verdict}")

    emit("")
    if failures:
        for f in failures:
            emit(f"FAIL {f}")
            print(f"FAIL {f}", file=sys.stderr)
        emit(f"{len(failures)} pair(s) below AA")
        rc = 1
    else:
        emit(f"all pairs >= {AA}:1 in both themes")
        rc = 0

    # Persist the audit: six months from now the only way to re-check a palette
    # decision is the recorded numbers from the day it shipped (W4).
    log = Path(__file__).with_suffix(".log")
    log.write_text(
        f"== contrast audit {datetime.now(timezone.utc).isoformat(timespec='seconds')} ==\n"
        + "\n".join(out)
        + "\n",
        encoding="utf-8",
    )
    print(f"\nlog written: {log.relative_to(Path(__file__).resolve().parent.parent)}")
    return rc


if __name__ == "__main__":
    sys.exit(main())
