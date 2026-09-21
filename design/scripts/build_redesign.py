#!/usr/bin/env python3
"""リデザインの見本(design/redesign/*.html)を capture_mock.js が書き出した JSON から、Penpot のデザインファイルを作る。
build_penpot.py の部品(Penpot・FileBuilder・Design・place)を使う。文字は Google Fonts の Noto Sans JP。
使い方は design/README.md の「リデザインのラフ」を参照。
"""
from __future__ import annotations

import argparse
import json
import sys
import uuid
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from build_penpot import Design, FileBuilder, Penpot, place  # noqa: E402

FONT = ("gfont-noto-sans-jp", "Noto Sans JP")
WEIGHTS = (400, 500, 700)
# Penpot は、行の位置(y)を、文字の下端(em ボックスの下)に合わせて描くので、行の上端から y までは、行の高さの 1.0 倍にする(描かれた画像との差で測った)
BASELINE = 1.0
WEIGHT_NAME = {"400": "標準", "500": "中", "700": "太字"}
# (キー, 画面の名前)。PC とモバイルの両方に、この順で並べる
SCREENS = [("shops", "ショップ一覧"), ("reviews", "レビュー一覧"), ("review-detail", "レビュー詳細"), ("shops-en", "ショップ一覧(English)"), ("states", "空・読み込み・エラー・404"), ("states-en", "空・読み込み・エラー・404(English)")]


def main() -> None:
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--base", default="http://localhost:9001")
    ap.add_argument("--email", required=True)
    ap.add_argument("--password-file", required=True)
    ap.add_argument("--captures", required=True, help="capture_mock.js の出力のディレクトリ")
    ap.add_argument("--name", default="hamburger-evaluation-redesign")
    ap.add_argument("--map-out")
    args = ap.parse_args()
    cap_dir = Path(args.captures)
    load = lambda name: json.loads((cap_dir / f"{name}.json").read_text())

    api = Penpot(args.base)
    prof = api.call("login-with-password", {"email": args.email, "password": Path(args.password_file).read_text().strip()})
    fid = api.call("create-file", {"name": args.name, "projectId": prof["defaultProjectId"]})["id"]
    page_value = api.call("get-file", {"id": fid})["data"]["pages"][0]
    fb = FileBuilder(api, fid)
    page_pc, page_mob, page_icons = str(uuid.uuid4()), str(uuid.uuid4()), str(uuid.uuid4())
    fb.add({"type": "mod-page", "id": page_value, "name": "値"})
    fb.add({"type": "add-page", "id": page_pc, "name": "画面(PC)"})
    fb.add({"type": "add-page", "id": page_mob, "name": "画面(モバイル)"})
    fb.add({"type": "add-page", "id": page_icons, "name": "評価アイコンの比較"})
    fb.flush()

    caps = {f"{k}-{size}": load(f"{k}-{size}") for k, _ in SCREENS for size in ("pc", "mobile")}
    tokens_cap = load("tokens")
    icons_cap = load("rating-icons")
    values = json.loads((cap_dir / "tokens-values.json").read_text())

    # 色のスタイル(見本の CSS の変数)。同じ値は、先に書いたほうにひも付ける
    colors: dict[str, str] = {}
    for t in values["tokens"]:
        cid = str(uuid.uuid4())
        fb.add({"type": "add-color", "color": {"id": cid, "name": t["name"], "color": t["value"], "opacity": 1, "path": "色"}})
        colors.setdefault(t["value"].lower(), cid)

    # 文字のスタイル(画面で使われている、大きさと太さの組み合わせ)
    def weight_of(w) -> str:
        return str(min(WEIGHTS, key=lambda x: abs(x - int(w))))
    first: dict[tuple, float] = {}
    for c in [*caps.values(), tokens_cap, icons_cap]:
        for n in c["nodes"]:
            if n["kind"] == "text":
                st = n["style"]
                first.setdefault((round(st["size"], 1), weight_of(st["weight"])), (st["lineHeight"] or st["size"] * 1.5) / st["size"])
    typos: dict[tuple, str] = {}
    for (size, weight), ratio in sorted(first.items()):
        tid = str(uuid.uuid4())
        typos[(size, weight)] = tid
        fb.add({"type": "add-typography", "typography": {
            "id": tid, "name": f"{size:g}px/{WEIGHT_NAME[weight]}", "path": "文字", "fontId": FONT[0], "fontFamily": FONT[1],
            "fontVariantId": "regular" if weight == "400" else weight, "fontSize": f"{size:g}", "fontWeight": weight, "fontStyle": "normal",
            "lineHeight": f"{ratio:.2f}", "letterSpacing": "0", "textTransform": "none"}})
    fb.flush()
    d = Design(fb, colors, typos, fid, font=FONT, weights=WEIGHTS, baseline=BASELINE, snap=True)

    # 値のページ
    frame = d.frame(page_value, "デザインの値", 0, 0, tokens_cap["width"], tokens_cap["height"], tokens_cap["bg"])
    place(d, page_value, tokens_cap["nodes"], 0, 0, frame)
    fb.flush()

    # 評価のバーガーの比較のページ
    frame = d.frame(page_icons, "評価アイコンの比較", 0, 0, icons_cap["viewport"], icons_cap["height"], icons_cap["bg"])
    place(d, page_icons, icons_cap["nodes"], 0, 0, frame)
    fb.flush()

    screen_map = []
    for size_key, page in (("pc", page_pc), ("mobile", page_mob)):
        x = 0
        for key, title in SCREENS:
            c = caps[f"{key}-{size_key}"]
            name = f"{title} / {'PC' if size_key == 'pc' else 'モバイル'}"
            frame = d.frame(page, name, x, 0, c["viewport"], c["height"], c["bg"])
            place(d, page, c["nodes"], x, 0, frame)
            fb.flush()
            screen_map.append({"screen": title, "key": key, "size": size_key, "page": "画面(PC)" if size_key == "pc" else "画面(モバイル)", "frame": name, "url": c["url"], "shapes": len(c["nodes"])})
            x += c["viewport"] + 160
    fb.flush()

    # 画像にするときの対象に、値のページと比較のページも入れる(render_penpot.js が、この一覧を使う)
    screen_map.append({"screen": "デザインの値", "key": "tokens", "size": "pc", "page": "値", "frame": "デザインの値", "url": "tokens.html", "shapes": len(tokens_cap["nodes"])})
    screen_map.append({"screen": "評価アイコンの比較", "key": "rating-icons", "size": "pc", "page": "評価アイコンの比較", "frame": "評価アイコンの比較", "url": "rating-icons.html", "shapes": len(icons_cap["nodes"])})
    info = {"fileId": fid, "name": args.name, "pages": {"値": page_value, "画面(PC)": page_pc, "画面(モバイル)": page_mob, "評価アイコンの比較": page_icons}, "screens": screen_map,
            "colors": len(values["tokens"]), "typographies": len(typos)}
    if args.map_out:
        Path(args.map_out).write_text(json.dumps(info, ensure_ascii=False, indent=1))
    print(json.dumps({k: (v if k != "screens" else len(v)) for k, v in info.items()}, ensure_ascii=False))


if __name__ == "__main__":
    main()
