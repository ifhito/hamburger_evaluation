#!/usr/bin/env python3
"""画面の構造(capture.js が書き出した JSON)から、Penpot のデザインファイルを作る。

いまの画面を、そのまま Penpot に再現するためのスクリプト。新しいデザインは作らない。
Penpot の API(update-file)に、画面ごとの枠(フレーム)・長方形・文字の変更を送る。
使い方は design/README.md の「いまの画面から作り直す」を参照。
"""
from __future__ import annotations

import argparse
import http.cookiejar
import json
import math
import re
import sys
import urllib.request
import uuid
from pathlib import Path

ROOT_ID = "00000000-0000-0000-0000-000000000000"
IDENTITY = {"a": 1.0, "b": 0.0, "c": 0.0, "d": 1.0, "e": 0.0, "f": 0.0}

# frontend/src/app/styles/globals.css の変数を、Penpot の色のスタイルにする。
# (CSS の変数名, Penpot での名前, 値)。値が同じ変数は、先に書いたほうにひも付ける。
COLOR_TOKENS = [
    ("--color-primary", "primary", "#e05c00"),
    ("--color-primary-hover", "primary-hover", "#c75400"),
    ("--color-danger", "danger", "#dc2626"),
    ("--color-danger-hover", "danger-hover", "#b91c1c"),
    ("--color-secondary-bg", "secondary-bg", "#f3f4f6"),
    ("--color-secondary-hover", "secondary-hover", "#e5e7eb"),
    ("--color-border", "border", "#d1d5db"),
    ("--color-text", "text", "#111827"),
    ("--color-text-muted", "text-muted", "#6b7280"),
    ("--color-error", "error", "#dc2626"),
    ("--color-success", "success", "#16a34a"),
    ("body の background", "background", "#fafafa"),
    ("(ヘッダーの文字・入力欄の背景)", "white", "#ffffff"),
]
RADIUS_PX = 6  # --radius: 6px
# Penpot は、行の位置(y)を「文字の下端の線(ベースライン)」として描く。ブラウザで測った行の上端から、その線までの割合(高さに対して)。
BASELINE = 0.8

# 部品にする見本。(部品の名前, 見本を取る画面, その画面から見本の長方形を選ぶ条件)
COMPONENTS = [
    ("Header/PC", "signin-pc", lambda n: n["kind"] == "rect" and n["name"] == "header"),
    ("Header/モバイル", "signin-mobile", lambda n: n["kind"] == "rect" and n["name"] == "header"),
    ("Button/Primary", "signin-pc", lambda n: n["kind"] == "rect" and n["tag"] == "button" and n["fill"] == "#e05c00"),
    ("Button/Secondary", "signup-sent-pc", lambda n: n["kind"] == "rect" and n["tag"] == "button" and n["fill"] == "#f3f4f6"),
    ("Button/Danger", "admin-shops-pc", lambda n: n["kind"] == "rect" and n["tag"] == "button" and n["fill"] == "#dc2626"),
    ("Input", "signin-pc", lambda n: n["kind"] == "rect" and n["tag"] == "input"),
    ("Textarea", "review-new-pc", lambda n: n["kind"] == "rect" and n["tag"] == "textarea"),
    ("Select", "review-new-pc", lambda n: n["kind"] == "rect" and n["tag"] == "select"),
    ("ErrorMessage", "signin-error-pc", lambda n: n["kind"] == "rect" and n["fill"] == "#fee2e2"),
    ("Card/Review", "reviews-pc", lambda n: n["kind"] == "rect" and n["name"] == "card"),
    ("Card/Review(ショップの詳細の中)", "shop-detail-pc", lambda n: n["kind"] == "rect" and n["name"] == "reviewCard"),
    ("Card/Review(プロフィールの中)", "profile-pc", lambda n: n["kind"] == "rect" and n["name"] == "reviewCard"),
    ("Card/Shop", "shops-pc", lambda n: n["kind"] == "rect" and n["name"] == "shopLink"),
    ("Row/管理のショップ", "admin-shops-pc", lambda n: n["kind"] == "rect" and n["name"] == "row"),
    ("Badge", "admin-shops-pc", lambda n: n["kind"] == "rect" and n["name"] == "badge"),
]


class Penpot:
    """Penpot の API(JSON)の最小の呼び出し口。"""

    def __init__(self, base: str) -> None:
        self.base = base.rstrip("/")
        self.opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))

    def call(self, name: str, params: dict) -> dict:
        req = urllib.request.Request(
            f"{self.base}/api/rpc/command/{name}",
            data=json.dumps(params).encode(),
            headers={"Content-Type": "application/json", "Accept": "application/json"},
            method="POST",
        )
        try:
            with self.opener.open(req) as res:
                return json.loads(res.read() or b"null")
        except urllib.error.HTTPError as e:
            raise SystemExit(f"Penpot API {name} が失敗: {e.code} {e.read().decode()[:1500]}")


class FileBuilder:
    """1 つのファイルに対する変更を、まとめて送る。"""

    def __init__(self, api: Penpot, file_id: str) -> None:
        self.api, self.file_id = api, file_id
        self.pending: list[dict] = []
        got = api.call("get-file", {"id": file_id})
        self.revn, self.vern = got["revn"], got["vern"]
        self.session = str(uuid.uuid4())

    def add(self, change: dict) -> None:
        self.pending.append(change)
        if len(self.pending) >= 300:
            self.flush()

    def flush(self) -> None:
        if not self.pending:
            return
        res = self.api.call("update-file", {"id": self.file_id, "sessionId": self.session, "revn": self.revn, "vern": self.vern, "changes": self.pending})
        lagged = res.get("lagged") or []
        self.revn = max([self.revn] + [x["revn"] for x in lagged])
        self.pending = []


def geom(x: float, y: float, w: float, h: float) -> dict:
    x, y, w, h = round(x, 2), round(y, 2), round(max(w, 0.01), 2), round(max(h, 0.01), 2)
    return {
        "x": x, "y": y, "width": w, "height": h, "rotation": 0,
        "selrect": {"x": x, "y": y, "width": w, "height": h, "x1": x, "y1": y, "x2": x + w, "y2": y + h},
        "points": [{"x": x, "y": y}, {"x": x + w, "y": y}, {"x": x + w, "y": y + h}, {"x": x, "y": y + h}],
        "transform": dict(IDENTITY), "transformInverse": dict(IDENTITY),
    }


class Design:
    def __init__(self, fb: FileBuilder, colors: dict[str, str], typos: dict[tuple, str], file_id: str,
                 font: tuple[str, str] = ("sourcesanspro", "sourcesanspro"), weights: tuple[int, ...] = (400, 700), baseline: float = BASELINE, snap: bool = False) -> None:
        # font は (Penpot のフォントの id, フォントの名前)、weights は使う太さ(CSS の太さは、いちばん近いものにする)、
        # baseline は BASELINE と同じ意味の割合(フォントごとに、描かれた画像と見比べて決める)
        self.fb, self.colors, self.typos, self.file_id = fb, colors, typos, file_id
        # snap は、長方形の位置と大きさを、整数の px に丸める(端が小数だと、Penpot の画像で、グラデーションの端に細い線が出るため)
        self.font, self.weights, self.baseline, self.snap = font, weights, baseline, snap

    # ---- 部品(色・文字のスタイルへのひも付けを含む) ----
    def fill(self, hexcolor: str, opacity: float = 1) -> dict:
        f = {"fillColor": hexcolor, "fillOpacity": round(opacity, 3)}
        ref = self.colors.get(hexcolor.lower())
        if ref:
            f["fillColorRefId"], f["fillColorRefFile"] = ref, self.file_id
        return f

    @staticmethod
    def gradient_fill(g: dict, w: float, h: float) -> dict:
        """CSS の linear-gradient(角度と色の止まり位置)を、Penpot のグラデーションにする。位置は、図形の大きさに対する 0〜1。"""
        t = math.radians(g["angle"])
        dx, dy = math.sin(t), -math.cos(t)
        length = abs(w * dx) + abs(h * dy)
        r4 = lambda v: round(v, 4)
        return {"fillColorGradient": {
            "type": "linear", "startX": r4(0.5 - dx * length / (2 * w)), "startY": r4(0.5 - dy * length / (2 * h)),
            "endX": r4(0.5 + dx * length / (2 * w)), "endY": r4(0.5 + dy * length / (2 * h)), "width": 1,
            "stops": [{"color": st["color"], "opacity": round(st["opacity"], 3), "offset": round(st["offset"], 4)} for st in g["stops"]]},
            "fillOpacity": 1}

    @staticmethod
    def snapped(x: float, y: float, w: float, h: float) -> tuple:
        x1, y1, x2, y2 = round(x), round(y), round(x + w), round(y + h)
        return x1, y1, x2 - x1, y2 - y1

    def add_obj(self, page: str, obj: dict) -> None:
        self.fb.add({"type": "add-obj", "id": obj["id"], "pageId": page, "parentId": obj["parentId"], "frameId": obj["frameId"], "obj": obj})

    def frame(self, page: str, name: str, x, y, w, h, bg: str | None, parent=ROOT_ID) -> str:
        fid = str(uuid.uuid4())
        obj = {"id": fid, "name": name, "type": "frame", "parentId": parent, "frameId": parent, "fills": [self.fill(bg)] if bg else [], "strokes": [], "shapes": [], **geom(x, y, w, h)}
        self.add_obj(page, obj)
        return fid

    def group(self, page: str, name: str, frame: str, bbox: tuple) -> str:
        gid = str(uuid.uuid4())
        self.add_obj(page, {"id": gid, "name": name, "type": "group", "parentId": frame, "frameId": frame, "shapes": [], **geom(*bbox)})
        return gid

    def rect(self, page: str, n: dict, ox: float, oy: float, frame: str, parent: str, name: str) -> None:
        r = min(n["h"], n["w"]) / 2
        rad = [round(min(v, r), 2) for v in n["radius"]]
        obj = {
            "id": str(uuid.uuid4()), "name": name, "type": "rect", "parentId": parent, "frameId": frame,
            "fills": [self.gradient_fill(n["gradient"], n["w"], n["h"])] if n.get("gradient") else ([self.fill(n["fill"], n["fillOpacity"])] if n["fill"] else []),
            "strokes": [{"strokeColor": n["stroke"]["color"], "strokeOpacity": round(n["stroke"]["opacity"], 3), "strokeWidth": n["stroke"]["w"], "strokeStyle": "solid", "strokeAlignment": "inner"}] if n["stroke"] else [],
            "r1": rad[0], "r2": rad[1], "r3": rad[2], "r4": rad[3],
            **(geom(*self.snapped(ox + n["x"], oy + n["y"], n["w"], n["h"])) if self.snap else geom(ox + n["x"], oy + n["y"], n["w"], n["h"])),
        }
        self.add_obj(page, obj)

    def text(self, page: str, n: dict, ox: float, oy: float, frame: str, parent: str) -> None:
        s = n["style"]
        weight = str(min(self.weights, key=lambda w: abs(w - int(s["weight"]))))
        run = {
            "text": n["text"], "fontFamily": self.font[1], "fontId": self.font[0], "fontSize": f"{round(s['size'], 1):g}",
            "fontStyle": "normal", "fontVariantId": "regular" if weight == "400" else weight, "fontWeight": weight,
            "lineHeight": str(round((s["lineHeight"] or s["size"] * 1.2) / s["size"], 2)), "letterSpacing": str(round(s["letterSpacing"], 2)),
            "textDecoration": "underline" if s["underline"] else "none", "textTransform": "none", "fills": [self.fill(s["color"])],
        }
        typo = self.typos.get((round(s["size"], 1), weight))
        if typo:
            run["typographyRefId"], run["typographyRefFile"] = typo, self.file_id
        align = {"start": "left", "left": "left", "center": "center", "right": "right", "end": "right"}.get(s["align"], "left")
        para = {"type": "paragraph", "textAlign": align, "children": [run], "fills": [self.fill(s["color"])]}
        content = {"type": "root", "children": [{"type": "paragraph-set", "children": [para]}]}
        single_line = n["h"] <= (s["lineHeight"] or s["size"] * 1.4) * 1.6
        # 表示用の行の位置。Penpot は、これがないと文字を描かない(編集すると、Penpot が計算し直す)。y は行の上端ではなくベースライン
        lines = n.get("lines") or [{"text": n["text"], "x": n["x"], "y": n["y"], "w": n["w"], "h": n["h"]}]
        position = [{
            "x": round(ox + ln["x"], 2), "y": round(oy + ln["y"] + ln["h"] * self.baseline, 2), "width": round(ln["w"], 2), "height": round(ln["h"], 2), "direction": "ltr",
            "fontFamily": run["fontFamily"], "fontId": run["fontId"], "fontSize": run["fontSize"], "fontStyle": "normal", "fontWeight": weight,
            "fontVariantId": run["fontVariantId"], "textDecoration": run["textDecoration"], "textTransform": "none", "letterSpacing": run["letterSpacing"],
            "fills": run["fills"], "text": ln["text"],
        } for ln in lines]
        obj = {
            "id": str(uuid.uuid4()), "name": n["text"][:40], "type": "text", "parentId": parent, "frameId": frame,
            "content": content, "growType": "auto-width" if single_line else "auto-height", "positionData": position, "fills": [], "strokes": [],
            **geom(ox + n["x"], oy + n["y"], n["w"] + (1 if single_line else 2), n["h"]),
        }
        self.add_obj(page, obj)


def est_width(text: str, size: float, weight: int = 400) -> float:
    """文字の幅の目安。値のページの文字は、実際に測った幅がないので、全角は 1 文字 = 文字の大きさ、半角は約 0.55 倍で数える。"""
    return sum(size if ord(c) > 0x2E7F else size * (0.58 if weight >= 600 else 0.52) for c in text)


def control_name(members: list[dict]) -> str:
    rect = next((m for m in members if m["kind"] == "rect"), None)
    tag = (rect or members[0])["tag"]
    if tag == "button":
        return {"#e05c00": "Button/Primary", "#f3f4f6": "Button/Secondary", "#dc2626": "Button/Danger"}.get(rect["fill"] if rect else None, "Button")
    return tag.capitalize()


def union(nodes: list[dict]) -> tuple:
    x1 = min(n["x"] for n in nodes); y1 = min(n["y"] for n in nodes)
    x2 = max(n["x"] + n["w"] for n in nodes); y2 = max(n["y"] + n["h"] for n in nodes)
    return (x1, y1, x2 - x1, y2 - y1)


def place(d: Design, page: str, nodes: list[dict], ox: float, oy: float, frame: str) -> None:
    """画面(または部品)の中身を置く。入力欄・ボタンなどは、1 つのグループにまとめる。"""
    members: dict[int, list[dict]] = {}
    for n in nodes:
        if n["ctl"]:
            members.setdefault(n["ctl"], []).append(n)
    groups: dict[int, str] = {}
    for n in nodes:
        parent = frame
        if n["ctl"]:
            if n["ctl"] not in groups:
                bx, by, bw, bh = union(members[n["ctl"]])
                groups[n["ctl"]] = d.group(page, control_name(members[n["ctl"]]), frame, (ox + bx, oy + by, bw, bh))
            parent = groups[n["ctl"]]
        if n["kind"] == "rect":
            d.rect(page, n, ox, oy, frame, parent, n["name"])
        else:
            d.text(page, n, ox, oy, frame, parent)


# 画面を並べる順(capture.js の routes と同じ)。
SCREEN_ORDER = ["signin", "signin-error", "signup", "signup-sent", "shops", "shop-detail", "reviews", "review-detail",
                "review-new", "review-edit", "profile", "profile-edit", "admin-shops", "admin-shop-edit"]


def load_captures(directory: Path) -> list[dict]:
    caps = [json.loads(p.read_text()) for p in sorted(directory.glob("*.json"))]
    return sorted(caps, key=lambda c: (c["size"] != "pc", SCREEN_ORDER.index(c["key"]) if c["key"] in SCREEN_ORDER else 99))


def main() -> None:
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--base", default="http://localhost:9001", help="Penpot の URL")
    ap.add_argument("--email", required=True)
    ap.add_argument("--password-file", required=True, help="パスワードを 1 行で書いたファイル(コマンドの履歴に残さないため)")
    ap.add_argument("--captures", required=True, help="capture.js が書き出した JSON のディレクトリ")
    ap.add_argument("--name", default="hamburger-evaluation")
    ap.add_argument("--map-out", help="画面と Penpot のフレーム名の対応を書き出す JSON のパス")
    args = ap.parse_args()

    api = Penpot(args.base)
    prof = api.call("login-with-password", {"email": args.email, "password": Path(args.password_file).read_text().strip()})
    file = api.call("create-file", {"name": args.name, "projectId": prof["defaultProjectId"]})
    fid = file["id"]
    got = api.call("get-file", {"id": fid})
    page_value = got["data"]["pages"][0]
    fb = FileBuilder(api, fid)

    # ページ: 値・部品・画面(PC)・画面(モバイル)
    page_comp, page_pc, page_mob = (str(uuid.uuid4()) for _ in range(3))
    fb.add({"type": "mod-page", "id": page_value, "name": "値"})
    for pid, name in ((page_comp, "部品"), (page_pc, "画面(PC)"), (page_mob, "画面(モバイル)")):
        fb.add({"type": "add-page", "id": pid, "name": name})
    fb.flush()

    caps = load_captures(Path(args.captures))

    # デザインの値: 色と文字のスタイル
    colors: dict[str, str] = {}
    for _var, name, value in COLOR_TOKENS:
        cid = str(uuid.uuid4())
        fb.add({"type": "add-color", "color": {"id": cid, "name": name, "color": value, "opacity": 1, "path": "色"}})
        colors.setdefault(value, cid)
    combos = sorted({(round(n["style"]["size"], 1), "700" if int(n["style"]["weight"]) >= 600 else "400") for c in caps for n in c["nodes"] if n["kind"] == "text"})
    typos: dict[tuple, str] = {}
    for size, weight in combos:
        tid = str(uuid.uuid4())
        typos[(size, weight)] = tid
        fb.add({"type": "add-typography", "typography": {
            "id": tid, "name": f"{size:g}px/{'太字' if weight == '700' else '標準'}", "path": "文字", "fontId": "sourcesanspro", "fontFamily": "sourcesanspro",
            "fontVariantId": "700" if weight == "700" else "regular", "fontSize": f"{size:g}", "fontWeight": weight, "fontStyle": "normal",
            "lineHeight": "1.6", "letterSpacing": "0", "textTransform": "none"}})
    fb.flush()
    d = Design(fb, colors, typos, fid)

    # 「値」のページ: 色の見本・文字の見本・角丸
    y0 = 90 + 4 * 170 + 20
    sample_h = [round(size * 1.4) + 12 for (size, _weight) in sorted(typos)]
    fr = d.frame(page_value, "デザインの値", 0, 0, 1240, y0 + 40 + sum(sample_h) + 70, "#ffffff")
    txt = lambda s, x, y, size=14, weight=400, color="#111827", h=20: d.text(page_value, {"text": s, "x": x, "y": y, "w": est_width(s, size, weight), "h": h, "style": {"size": size, "weight": str(weight), "color": color, "lineHeight": size * 1.4, "align": "left", "letterSpacing": 0, "underline": False}}, 0, 0, fr, fr)
    txt("デザインの値(いまの画面の globals.css の変数を、Penpot の色・文字のスタイルにしたもの)", 40, 30, 20, 700, h=28)
    for i, (var, name, value) in enumerate(COLOR_TOKENS):
        cx, cy = 40 + (i % 4) * 290, 90 + (i // 4) * 170
        d.rect(page_value, {"x": cx, "y": cy, "w": 250, "h": 96, "fill": value, "fillOpacity": 1, "stroke": {"w": 1, "color": "#d1d5db", "opacity": 1}, "radius": [RADIUS_PX] * 4}, 0, 0, fr, fr, f"色/{name}")
        txt(f"{name}  {value}", cx, cy + 104, 14, 700)
        txt(var, cx, cy + 126, 12, 400, "#6b7280", h=16)
    txt("文字のスタイル(Penpot に用意された sourcesanspro で代用。実際の画面は OS の標準のフォント)", 40, y0, 16, 700, h=24)
    yy = y0 + 40
    for ((size, weight), _tid), row_h in zip(sorted(typos.items()), sample_h):
        txt(f"{size:g}px {'太字' if weight == '700' else '標準'}: Hamburger Evaluation ハンバーガーの評価", 40, yy, size, int(weight), h=size * 1.4)
        yy += row_h
    txt(f"角丸(--radius): {RADIUS_PX}px / ページの背景: #fafafa / フォント(--font-sans): system-ui, -apple-system, sans-serif", 40, yy + 20, 14, 400, "#6b7280")
    fb.flush()

    # 部品のページ
    comp_map = []
    cx, cy = 40, 60
    row_h = 0
    for name, cap_key, pred in COMPONENTS:
        cap = next((c for c in caps if f"{c['key']}-{c['size']}" == cap_key), None)
        seed = next((n for n in cap["nodes"] if pred(n)), None) if cap else None
        if not seed:
            print(f"部品 {name}: 見本が見つからないため飛ばします", file=sys.stderr)
            continue
        if seed["ctl"]:
            inside = [n for n in cap["nodes"] if n["ctl"] == seed["ctl"]]
        else:
            inside = [n for n in cap["nodes"] if n["x"] >= seed["x"] - 1 and n["y"] >= seed["y"] - 1 and n["x"] + n["w"] <= seed["x"] + seed["w"] + 1 and n["y"] + n["h"] <= seed["y"] + seed["h"] + 1]
        if cx + seed["w"] > 1400:
            cx, cy, row_h = 40, cy + row_h + 80, 0
        board = d.frame(page_comp, name, cx, cy, seed["w"], seed["h"], None)
        rel = [{**n, "x": n["x"] - seed["x"], "y": n["y"] - seed["y"], **({"lines": [{**ln, "x": ln["x"] - seed["x"], "y": ln["y"] - seed["y"]} for ln in n["lines"]]} if n.get("lines") else {})} for n in inside]
        # 部品の中の入力欄・ボタンは、1 つの部品として扱うので、グループにはしない
        for n in rel:
            n = {**n, "ctl": 0}
            (d.rect(page_comp, n, cx, cy, board, board, n["name"]) if n["kind"] == "rect" else d.text(page_comp, n, cx, cy, board, board))
        fb.flush()
        comp = str(uuid.uuid4())
        got = api.call("get-file", {"id": fid})
        objs = got["data"]["pagesIndex"][page_comp]["objects"]
        shapes = [objs[board]] + [objs[c] for c in objs[board].get("shapes", [])]
        fb.revn, fb.vern = got["revn"], got["vern"]
        fb.add({"type": "add-component", "id": comp, "name": name, "path": "", "mainInstanceId": board, "mainInstancePage": page_comp, "shapes": shapes})
        fb.add({"type": "mod-obj", "id": board, "pageId": page_comp, "operations": [
            {"type": "set", "attr": "componentId", "val": comp, "ignoreTouched": True}, {"type": "set", "attr": "componentFile", "val": fid, "ignoreTouched": True},
            {"type": "set", "attr": "componentRoot", "val": True, "ignoreTouched": True}, {"type": "set", "attr": "mainInstance", "val": True, "ignoreTouched": True}]})
        fb.flush()
        comp_map.append({"name": name, "from": cap_key, "size": [round(seed["w"]), round(seed["h"])], "shapes": len(rel)})
        cx += seed["w"] + 60
        row_h = max(row_h, seed["h"])

    # 画面のページ
    screen_map = []
    for size_key, page in (("pc", page_pc), ("mobile", page_mob)):
        group = [c for c in caps if c["size"] == size_key]
        x = y = 0
        row_h = 0
        per_row = 3 if size_key == "pc" else 6
        for i, c in enumerate(group):
            w, h = c["viewport"], c["height"]
            if i and i % per_row == 0:
                x, y, row_h = 0, y + row_h + 200, 0
            name = f"{c['title']} / {'PC' if size_key == 'pc' else 'モバイル'}"
            frame = d.frame(page, name, x, y, w, h, c["bg"])
            place(d, page, c["nodes"], x, y, frame)
            fb.flush()
            screen_map.append({"screen": c["title"], "key": c["key"], "size": size_key, "page": "画面(PC)" if size_key == "pc" else "画面(モバイル)", "frame": name, "url": c["url"], "shapes": len(c["nodes"])})
            x += w + 120
            row_h = max(row_h, h)
    fb.flush()

    info = {"fileId": fid, "name": args.name, "pages": {"値": page_value, "部品": page_comp, "画面(PC)": page_pc, "画面(モバイル)": page_mob}, "screens": screen_map, "components": comp_map, "colors": len(COLOR_TOKENS), "typographies": len(typos)}
    if args.map_out:
        Path(args.map_out).write_text(json.dumps(info, ensure_ascii=False, indent=1))
    print(json.dumps({k: (v if k not in ("screens", "components") else len(v)) for k, v in info.items()}, ensure_ascii=False))


if __name__ == "__main__":
    main()
