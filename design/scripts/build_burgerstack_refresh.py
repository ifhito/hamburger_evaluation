#!/usr/bin/env python3
"""Create Penpot boards from read-only measurements of the actual React screens.
No page layout or rating icon is invented here. Original captures are the source.
"""
import argparse,json,re,urllib.request,uuid
from pathlib import Path
from build_penpot import Penpot,FileBuilder,Design,geom
ROOT=Path(__file__).resolve().parents[2]
REFRESH=ROOT/'design/refresh'
NAMES={'shops':'ショップ一覧','burgers':'バーガー一覧','reviews':'レビュー一覧','shop':'ショップ詳細','burger':'バーガー詳細','review':'レビュー詳細','profile':'ユーザー詳細','empty':'ユーザー詳細・レビューなし','apps':'本人プロフィール・接続済みアプリ','about':'Burger Stackについて'}

def upload(api,fid):
 boundary=uuid.uuid4().hex;data=b''
 for k,v in {'file-id':fid,'name':'サンプル写真','is-local':'true'}.items():data+=f'--{boundary}\r\nContent-Disposition: form-data; name="{k}"\r\n\r\n{v}\r\n'.encode()
 data+=f'--{boundary}\r\nContent-Disposition: form-data; name="content"; filename="sample.jpg"\r\nContent-Type: image/jpeg\r\n\r\n'.encode()+(REFRESH/'assets/sample-burger.jpg').read_bytes()+f'\r\n--{boundary}--\r\n'.encode()
 req=urllib.request.Request(api.base+'/api/rpc/command/upload-file-media-object',data=data,headers={'Content-Type':'multipart/form-data; boundary='+boundary,'Accept':'application/json'})
 obj=json.loads(api.opener.open(req).read());return {k:obj[k] for k in ['id','width','height','mtype']}

class BufferedBuilder(FileBuilder):
 def add(self,c):self.pending.append(c)

# Convert the rendered SVG coordinates without changing the icon geometry.
def lerp(a,b,t):return (a[0]+(b[0]-a[0])*t,a[1]+(b[1]-a[1])*t)
def curves(path):
 tokens=re.findall(r'[MLC]|-?(?:\d*\.\d+|\d+)(?:[eE][-+]?\d+)?',path);i=0;pos=(0,0);out=[]
 assert not set(re.findall(r'[a-df-zA-DF-Z]',path))-set('MLC'),path
 while i<len(tokens):
  cmd=tokens[i];i+=1
  if cmd in 'ML':
   end=tuple(map(float,tokens[i:i+2]));i+=2
   if cmd=='L':out.append((pos,lerp(pos,end,1/3),lerp(pos,end,2/3),end))
   pos=end
  else:
   vals=list(map(float,tokens[i:i+6]));i+=6;out.append((pos,tuple(vals[:2]),tuple(vals[2:4]),tuple(vals[4:6])));pos=out[-1][-1]
 return out

class MeasuredDesign(Design):
 def icon(self,page,n,ox,oy,frame,parent):
  ic=n['icon'];vw,vh=ic['vb'];vx,vy=ic.get('origin',[0,0]);scale=min(n['w']/vw,n['h']/vh)
  dx=ox+n['x']+(n['w']-vw*scale)/2-vx*scale;dy=oy+n['y']+(n['h']-vh*scale)/2-vy*scale
  group=self.group(page,n['name'],frame,(ox+n['x'],oy+n['y'],n['w'],n['h']))
  def transform(p):return (dx+p[0]*scale,dy+p[1]*scale)
  for shape in ic['shapes']:
   for painted in [curves(shape['d'])]:
    content=[];points=[]
    for i,c in enumerate(painted):
     a,b,e,f=map(transform,c);points.extend([a,b,e,f])
     if i==0:content.append({'command':'move-to','params':{'x':a[0],'y':a[1]}})
     content.append({'command':'curve-to','params':{'c1x':b[0],'c1y':b[1],'c2x':e[0],'c2y':e[1],'x':f[0],'y':f[1]}})
    xs,ys=zip(*points);sid=str(uuid.uuid4());stroke=[] if shape.get('strokeNone') else [{'strokeColor':shape['stroke'],'strokeOpacity':1,'strokeWidth':shape['sw']*scale,'strokeStyle':'dashed' if shape.get('dash') else 'solid','strokeAlignment':'center',**({'strokeDash':float(re.findall(r'[\d.]+',shape['dash'])[0])*scale,'strokeGap':float(re.findall(r'[\d.]+',shape['dash'])[1])*scale} if shape.get('dash') else {}),**({'strokeCapStart':'round','strokeCapEnd':'round'} if shape.get('round') else {})}]
    self.add_obj(page,{'id':sid,'name':n['name']+' / '+shape['part'],'type':'path','parentId':group,'frameId':frame,'content':content,'fills':[] if not shape['fill'] else [self.fill(shape['fill']['color'])],'strokes':stroke,**geom(min(xs),min(ys),max(xs)-min(xs),max(ys)-min(ys))})
 def image(self,page,n,ox,oy,frame,media):
  self.add_obj(page,{'id':str(uuid.uuid4()),'name':n['name'],'type':'rect','parentId':frame,'frameId':frame,'fills':[{'fillImage':{**media,'keepAspectRatio':n['objectFit']=='cover'},'fillOpacity':1}],'strokes':[],**{f'r{i+1}':r for i,r in enumerate(n['radius'])},**geom(ox+n['x'],oy+n['y'],n['w'],n['h'])})

def main():
 parser=argparse.ArgumentParser();parser.add_argument('--email',required=True);parser.add_argument('--password-file',required=True);parser.add_argument('--only',help='Comma-separated capture keys for a verification file');args=parser.parse_args()
 api=Penpot('http://localhost:9001');profile=api.call('login-with-password',{'email':args.email,'password':Path(args.password_file).read_text().strip()})
 fid=api.call('create-file',{'name':'BurgerStack — 現行画面＋追加機能','projectId':profile['defaultProjectId']})['id'];data=api.call('get-file',{'id':fid})['data'];initial=data['pages'][0]
 media=upload(api,fid);fb=BufferedBuilder(api,fid);d=MeasuredDesign(fb,{}, {},fid,font=('gfont-noto-sans-jp','Noto Sans JP'),weights=tuple(range(100,1000,100)),baseline=1.0,snap=False);mapping=[]
 for suffix,label in [('pc','PC'),('mobile','モバイル')]:
  page=initial if suffix=='pc' else str(uuid.uuid4());fb.add({'type':'mod-page' if suffix=='pc' else 'add-page','id':page,'name':'現行画面＋追加機能 / '+label});fb.flush();x=0
  for key,name in NAMES.items():
   capture_key=key+'-'+suffix
   if args.only and capture_key not in args.only.split(','):continue
   c=json.loads((REFRESH/'captures'/f'{capture_key}.json').read_text());frame=d.frame(page,name+' / '+label,x,0,c['width'],c['height'],c['bg'])
   for node in c['nodes']:
    if node['kind']=='rect':d.rect(page,node,x,0,frame,frame,node['name'])
    elif node['kind']=='icon':d.icon(page,node,x,0,frame,frame)
    elif node['kind']=='image':d.image(page,node,x,0,frame,media)
    else:d.text(page,node,x,0,frame,frame)
   fb.flush();mapping.append({'key':capture_key,'page':page,'frame':frame,'width':c['width'],'height':c['height']});x+=c['width']+160
  print(label+' imported',flush=True)
 (REFRESH/'boards.json').write_text(json.dumps({'fileId':fid,'boards':mapping},ensure_ascii=False,indent=2)+'\n')
 req=urllib.request.Request(api.base+'/api/rpc/command/export-binfile',data=json.dumps({'fileId':fid,'version':3,'includeLibraries':False,'embedAssets':True}).encode(),headers={'Content-Type':'application/json','Accept':'application/json'})
 result=api.opener.open(req).read().decode();(Path(args.password_file).parent/'export-result.txt').write_text(result);urls=re.findall(r'https?[^"\s]+',result)
 if not urls:raise RuntimeError('No export URL returned')
 (ROOT/'design/files/burgerstack-refresh-2026-09.penpot').write_bytes(api.opener.open(urls[-1]).read());print('Saved',fid,flush=True)
if __name__=='__main__':main()
