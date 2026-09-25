#!/usr/bin/env python3
"""Validate that the published Penpot archive, board map and PNG previews agree."""
import json
import struct
import zipfile
import zlib
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
REFRESH = ROOT / 'design/refresh'
boards = json.loads((REFRESH / 'boards.json').read_text())['boards']
assert len(boards) == 20
assert len({b['key'] for b in boards}) == 20
assert {b['width'] for b in boards} == {375, 1280}
with zipfile.ZipFile(ROOT / 'design/files/burgerstack-refresh-2026-09.penpot') as archive:
    assert archive.testzip() is None
    page_ids = {b['page'] for b in boards}
    shapes = {}
    pages = []
    for name in archive.namelist():
        if '/pages/' not in name or not name.endswith('.json'):
            continue
        if name.count('/') == 3:
            pages.append(json.loads(archive.read(name)))
        if any(page_id in name for page_id in page_ids):
            obj = json.loads(archive.read(name))
            if isinstance(obj, dict) and 'type' in obj:
                shapes[obj['id']] = obj
    assert len(pages) == 2  # Faithful PC/mobile pages, without the superseded proposal.
    for board in boards:
        shape = shapes[board['frame']]
        assert shape['type'] == 'frame'
        assert shape['width'] == board['width']
        assert abs(shape['height'] - board['height']) < .01
    rating_paths = [s for s in shapes.values() if s['type'] == 'path' and '点の評価 /' in s['name']]
    assert rating_paths
    assert all(not s.get('fills') for s in rating_paths)
    dashed = [stroke for s in rating_paths for stroke in s['strokes'] if stroke['strokeStyle'] == 'dashed']
    assert dashed and all(s['strokeColor'] == '#dedcd6' and s['strokeDash'] > 0 and s['strokeGap'] > 0 for s in dashed)
    text = '\n'.join(json.dumps(s.get('content'), ensure_ascii=False)
                     for s in shapes.values() if s['type'] == 'text')
    for term in ['まだレビューがありません。', '食べた日', 'に投稿', 'Burger Stackについて',
                 '接続済みのアプリ', '世の中に、もっと']:
        assert term in text, term

gallery = (REFRESH / 'index.html').read_text()
for board in boards:
    capture = json.loads((REFRESH / 'captures' / (board['key'] + '.json')).read_text())
    assert (capture['width'], capture['height']) == (board['width'], board['height'])
    # The six open rating paths must preserve their solid/dashed thin strokes.
    ratings = [n for n in capture['nodes'] if n['kind'] == 'icon' and n['name'] != 'サービスロゴ' and len(n['icon']['shapes']) == 6]
    if board['key'].startswith('reviews-'):
        assert ratings
        assert any(p['dash'] for n in ratings for p in n['icon']['shapes'])
        assert all(p['fill'] is None and p['round'] for n in ratings for p in n['icon']['shapes'])
    key, suffix = board['key'].rsplit('-', 1)
    for filename in [board['key'] + '.png', key + '-source-' + suffix + '.png']:
        relative = 'previews/' + filename
        assert relative in gallery
        data = (REFRESH / relative).read_bytes()
        assert data[:8] == b'\x89PNG\r\n\x1a\n'
        width, height = struct.unpack('>II', data[16:24])
        assert width == board['width'], (filename, width, board['width'])
        assert abs(height - board['height']) <= 1, (filename, height, board['height'])
        offset = 8
        while offset < len(data):
            size = struct.unpack('>I', data[offset:offset + 4])[0]
            chunk = data[offset + 4:offset + 8 + size]
            expected = struct.unpack('>I', data[offset + 8 + size:offset + 12 + size])[0]
            assert zlib.crc32(chunk) & 0xffffffff == expected
            offset += size + 12
        assert offset == len(data)
print('PASS: Penpot ZIP, 2 pages, 20 boards/captures, key copy, rating paths, gallery links and 40 PNG CRCs/dimensions')
