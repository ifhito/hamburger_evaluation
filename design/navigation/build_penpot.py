#!/usr/bin/env python3
"""計測済みのナビ案を新規Penpotファイルとして作成し、編集用ファイルを書き出す。"""
import argparse
import json
import re
import sys
import urllib.request
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / 'design/scripts'))
from build_penpot import Penpot
from build_burgerstack_refresh import BufferedBuilder, MeasuredDesign, upload

NAMES = {'home-pc':'PC / ヘッダー・フッター', 'home-mobile':'スマホ / ホーム', 'search-mobile':'スマホ / 探す', 'record-mobile':'スマホ / 記録する', 'profile-mobile':'スマホ / マイページ', 'footer-mobile':'スマホ / フッター末尾', 'guest-mobile':'スマホ / 未ログイン', 'compact-mobile':'スマホ / 320px', 'record-hover-pc':'PC / 記録ボタンのホバー'}

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--email', required=True)
    parser.add_argument('--password-file', required=True)
    args = parser.parse_args()
    api = Penpot('http://localhost:9001')
    profile = api.call('login-with-password', {'email':args.email, 'password':Path(args.password_file).read_text().strip()})
    fid = api.call('create-file', {'name':'BurgerStack — ナビゲーション設計 #240', 'projectId':profile['defaultProjectId']})['id']
    pid = api.call('get-file', {'id':fid})['data']['pages'][0]
    fb = BufferedBuilder(api, fid)
    fb.add({'type':'mod-page','id':pid,'name':'ヘッダー・フッター・下部ナビ'})
    media = upload(api, fid)
    design = MeasuredDesign(fb, {}, {}, fid, font=('gfont-noto-sans-jp','Noto Sans JP'), weights=tuple(range(100,1000,100)), baseline=1.0)
    boards=[]
    for i,(key,name) in enumerate(NAMES.items()):
        c=json.loads((ROOT/'design/navigation/captures'/f'{key}.json').read_text())
        x=0 if i==0 else 1440+(i-1)%4*520
        y=0 if i<5 else 1080
        frame=design.frame(pid,name,x,y,c['width'],c['height'],c['bg'])
        for node in c['nodes']:
            if node['kind']=='rect':design.rect(pid,node,x,y,frame,frame,node['name'])
            elif node['kind']=='image':design.image(pid,node,x,y,frame,media)
            elif node['kind']=='icon':
                # 離れた線分を結ばず、SVGの各Mから始まるパスを保つ。
                node['icon']['shapes']=[{**shape,'d':part} for shape in node['icon']['shapes'] for part in re.split(r'(?=M)',shape['d']) if part.strip()]
                design.icon(pid,node,x,y,frame,frame)
            else:design.text(pid,node,x,y,frame,frame)
        fb.flush()
        boards.append({'key':key,'name':name,'page':pid,'frame':frame,'width':c['width'],'height':c['height']})
    mapping={'fileId':fid,'projectId':profile['defaultProjectId'],'boards':boards}
    (ROOT/'design/navigation/boards.json').write_text(json.dumps(mapping,ensure_ascii=False,indent=2)+'\n')
    req=urllib.request.Request(api.base+'/api/rpc/command/export-binfile',data=json.dumps({'fileId':fid,'version':3,'includeLibraries':False,'embedAssets':True}).encode(),headers={'Content-Type':'application/json','Accept':'application/json'})
    result=api.opener.open(req).read().decode()
    urls=re.findall(r'https?[^"\s]+',result)
    if not urls:raise RuntimeError('Penpotの書き出しURLがありません')
    (ROOT/'design/files/burgerstack-navigation.penpot').write_bytes(api.opener.open(urls[-1]).read())
    print(json.dumps({'fileId':fid,'boards':len(boards)}))

if __name__=='__main__':main()
