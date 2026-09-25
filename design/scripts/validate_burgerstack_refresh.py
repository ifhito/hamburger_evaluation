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
    assert len(pages) == 20  # Existing 18 reference pages plus the new PC/mobile pages.
    for board in boards:
        shape = shapes[board['frame']]
        assert shape['type'] == 'frame'
        assert shape['width'] == board['width']
        assert abs(shape['height'] - board['height']) < .01
    text = '\n'.join(json.dumps(s.get('content'), ensure_ascii=False)
                     for s in shapes.values() if s['type'] == 'text')
    for term in ['何も食べていません', '食べた日', '投稿日', 'Burger Stackについて',
                 '接続済みのアプリ', '世の中に、もっと']:
        assert term in text, term

gallery = (REFRESH / 'index.html').read_text()
for board in boards:
    relative = 'previews/' + board['key'] + '.png'
    assert relative in gallery
    data = (REFRESH / relative).read_bytes()
    assert data[:8] == b'\x89PNG\r\n\x1a\n'
    width, height = struct.unpack('>II', data[16:24])
    assert width == board['width']
    # Exporter rounds fractional frame bounds to whole image pixels.
    assert abs(height - board['height']) <= 1
    offset = 8
    while offset < len(data):
        size = struct.unpack('>I', data[offset:offset + 4])[0]
        chunk = data[offset + 4:offset + 8 + size]
        expected = struct.unpack('>I', data[offset + 8 + size:offset + 12 + size])[0]
        assert zlib.crc32(chunk) & 0xffffffff == expected
        offset += size + 12
    assert offset == len(data)
print('PASS: Penpot ZIP, 20 boards, key copy, gallery links and 20 PNG CRCs/dimensions')
