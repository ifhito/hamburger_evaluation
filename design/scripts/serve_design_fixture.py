#!/usr/bin/env python3
"""Local-only sample API for rendering the unchanged React screens in Penpot captures."""
import json
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from urllib.parse import urlparse, parse_qs
ROOT=Path(__file__).resolve().parents[2]
USER={'id':'u1','username':'HAL 1984','email':'fixture@example.test','can_moderate':False}
SHOPS=[{'id':'s1','name':'サンプルバーガー 下北沢店','status':'active','photo_url':'/photos/fixture.jpg','average_rating':4.5,'review_count':2,'map_url':None,'closed_at':None}, {'id':'s2','name':'サンプルバーガー 吉祥寺店','status':'active','photo_url':None,'average_rating':None,'review_count':0,'map_url':None,'closed_at':None}]
BURGERS=[{'id':'b1','name':'チーズバーガー','photo_url':'/photos/fixture.jpg','shop':{'id':'s1','name':SHOPS[0]['name']},'average_rating':4.5,'weighted_score':4.2,'review_count':2},{'id':'b2','name':'クラシックバーガー','photo_url':None,'shop':{'id':'s2','name':SHOPS[1]['name']},'average_rating':4,'weighted_score':4,'review_count':1}]
REVIEWS=[{'id':'r1','rating':4,'comment':'香ばしいパティと、とろけるチーズ。\n最後の一口までおいしくいただきました。','photo_url':'/photos/fixture.jpg','created_at':'2026-09-25T09:00:00Z','visited_at':'2026-09-24','shop':BURGERS[0]['shop'],'user':{'id':'u1','username':'HAL 1984'},'burger':BURGERS[0],'can_edit':True},{'id':'r2','rating':5,'comment':'バンズとパティの相性がよく、また食べに来たい一品でした。','photo_url':None,'created_at':'2026-09-24T09:00:00Z','visited_at':'2026-09-23','shop':BURGERS[0]['shop'],'user':{'id':'u3','username':'サンプルユーザー'},'burger':BURGERS[0],'can_edit':False}]
class Handler(BaseHTTPRequestHandler):
 def send(self,value,status=200):
  data=json.dumps(value,ensure_ascii=False).encode();self.send_response(status);self.send_header('Content-Type','application/json');self.send_header('X-Has-More','false');self.send_header('Content-Length',str(len(data)));self.end_headers();self.wfile.write(data)
 def do_POST(self):
  if self.path=='/login':self.send({**USER,'token':'local-design-fixture-token'})
  else:self.send({'error':'Read-only design fixture'},405)
 def do_GET(self):
  p=urlparse(self.path);q=parse_qs(p.query);route=p.path
  if route=='/photos/fixture.jpg':
   data=(ROOT/'design/refresh/assets/sample-burger.jpg').read_bytes();self.send_response(200);self.send_header('Content-Type','image/jpeg');self.end_headers();self.wfile.write(data);return
  if route=='/meta':self.send({'rating':{'min':1,'max':5},'photo':{'max_edge':2048,'max_bytes':10485760},'text':{'review_comment_max_chars':2000,'burger_name_max_chars':100,'shop_name_max_chars':100,'username_max_chars':50,'bio_max_chars':1000,'moderation_note_max_chars':1000},'password':{'min_bytes':8,'max_bytes':72},'login_providers':['google']})
  elif route=='/me':self.send(USER)
  elif route=='/me/identities':self.send({'identities':[]})
  elif route=='/oauth/grants':self.send([{'id':'g1','client_id':'sample-client','client_name':'ChatGPT','scopes':[{'name':'hamburger:read','description':'Read access','writes':False},{'name':'hamburger:write','description':'Write access','writes':True}],'created_at':'2026-09-24T09:00:00Z','updated_at':'2026-09-24T09:00:00Z'}])
  elif route=='/shops':self.send(SHOPS)
  elif route.startswith('/shops/'):
   shop=next((s for s in SHOPS if s['id']==route.split('/')[-1]),None)
   self.send({**shop,'creator':{'id':'u1','username':'HAL 1984'},'moderation_note':None,'reviews':REVIEWS if shop['id']=='s1' else [],'can_review':True} if shop else {'error':'Not found'},200 if shop else 404)
  elif route=='/burgers':self.send(BURGERS)
  elif route.startswith('/burgers/'):
   b=next((b for b in BURGERS if b['id']==route.split('/')[-1]),None);self.send({**b,'shops':[b['shop']]} if b else {'error':'Not found'},200 if b else 404)
  elif route=='/reviews':self.send([] if q.get('user_id')==['u2'] else [{**r,'user':{'id':'u3','username':'サンプルユーザー'},'can_edit':False} if q.get('user_id')==['u3'] else r for r in REVIEWS])
  elif route.startswith('/reviews/'):
   r=next((r for r in REVIEWS if r['id']==route.split('/')[-1]),None);self.send({**r,'can_review':True} if r else {'error':'Not found'},200 if r else 404)
  elif route.startswith('/users/'):
   uid=route.split('/')[-1];self.send({'id':uid,'username':'HAL 1984' if uid=='u1' else 'サンプルユーザー','bio':'ハンバーガーが好きです。','email':'fixture@example.test','admin':False,'can_edit':uid=='u1'})
  else:self.send({'error':'Not found'},404)
 def log_message(self,*args):pass
if __name__=='__main__':
 print('Design fixture API: http://127.0.0.1:18091',flush=True);ThreadingHTTPServer(('127.0.0.1',18091),Handler).serve_forever()
