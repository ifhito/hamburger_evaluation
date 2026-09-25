#!/usr/bin/env python3
"""Append editable BurgerStack refresh boards to an imported Penpot file.
Local-only API. Credentials are supplied through a private password file.
Existing reference pages are preserved. Coordinates and copy live in this file.
"""
import argparse,base64,html,json,re,urllib.request,uuid
from pathlib import Path
from build_penpot import Penpot,FileBuilder,Design,geom,est_width
ROOT=Path(__file__).resolve().parents[2]
ASSETS=ROOT/'design/refresh/assets'
OUT=ROOT/'design/refresh/previews'
INK='#171714';MUTED='#65645d';LINE='#dfddd4';YELLOW='#e9b824';CREAM='#fbf7e9';WHITE='#ffffff'
ICONS=json.loads((ASSETS/'rating-icons.json').read_text())
MARK=(ASSETS/'burgerstack-mark.svg').read_text()
PHOTO='data:image/jpeg;base64,'+base64.b64encode((ASSETS/'sample-burger.jpg').read_bytes()).decode()

def multipart(api,cmd,fields,filename,data,part='content',mime='image/jpeg'):
    bound=uuid.uuid4().hex;b=b''
    for k,v in fields.items():b+=f'--{bound}\r\nContent-Disposition: form-data; name="{k}"\r\n\r\n{v}\r\n'.encode()
    b+=f'--{bound}\r\nContent-Disposition: form-data; name="{part}"; filename="{filename}"\r\nContent-Type: {mime}\r\n\r\n'.encode()+data+f'\r\n--{bound}--\r\n'.encode()
    req=urllib.request.Request(api.base+'/api/rpc/command/'+cmd,data=b,headers={'Content-Type':'multipart/form-data; boundary='+bound,'Accept':'application/json'})
    return api.opener.open(req).read()

class BufferedFileBuilder(FileBuilder):
    def add(self, change):
        self.pending.append(change)

class Board:
    def __init__(self,d,page,name,key,w,h,x):
        self.d=d;self.page=page;self.w=w;self.h=h;self.x=x;self.key=key;self.svg=[];self.nodes=0;self.bottom=0
        self.frame=d.frame(page,name,x,0,w,h,WHITE);self.mobile=w<600;self.pad=24 if self.mobile else 72;self.cw=w-2*self.pad
    def rect(self,x,y,w,h,fill=WHITE,r=0,stroke=None,name='Panel'):
        self.bottom=max(self.bottom,y+h)
        self.d.rect(self.page,{'x':x,'y':y,'w':w,'h':h,'fill':fill,'fillOpacity':1,'radius':[r]*4,'stroke':{'color':stroke,'opacity':1,'w':1} if stroke else None},self.x,0,self.frame,self.frame,name)
        self.svg.append(f'<rect x="{x}" y="{y}" width="{w}" height="{h}" rx="{r}" fill="{fill}" stroke="{stroke or "none"}"/>')
    def text(self,text,x,y,w,size=16,weight=400,color=INK,underline=False,line=None):
        lh=line or size*1.6;lines=[]
        for para in text.split('\n'):
            s='';units=0
            for ch in para:
                u=1 if ord(ch)>0x2e7f else .56
                if (units+u)*size>w and s:lines.append(s);s='';units=0
                s+=ch;units+=u
            lines.append(s)
        for i,s in enumerate(lines):
            yy=y+i*lh
            self.bottom=max(self.bottom,yy+lh)
            n={'text':s,'x':x,'y':yy,'w':min(w,est_width(s,size,weight)),'h':lh,'style':{'size':size,'weight':weight,'lineHeight':lh,'letterSpacing':0,'underline':underline,'align':'left','color':color}}
            self.d.text(self.page,n,self.x,0,self.frame,self.frame)
            self.svg.append(f'<text x="{x}" y="{yy+size}" font-size="{size}" font-weight="{weight}" fill="{color}" text-decoration="{"underline" if underline else "none"}">{html.escape(s)}</text>')
        return y+len(lines)*lh
    def button(self,label,x,y,w=164,primary=True):
        self.rect(x,y,w,46,YELLOW if primary else WHITE,8,None if primary else LINE,label)
        self.text(label,x+16,y+10,w-28,14,700)
    def rating(self,x,y,value=4.5):
        ic=ICONS[str(value)];self.d.icon(self.page,{'name':'評価 '+str(value),'x':x,'y':y,'w':48,'h':40,'icon':ic},self.x,0,self.frame,self.frame)
        # Preview uses the same path geometry as the editable Penpot icon.
        parts=[]
        for i,s in enumerate(ic['shapes']):
            f=s['fill'];fill='none' if not f else f.get('color','#fff')
            if f and 'axis' in f:
                gid=f'g{uuid.uuid4().hex}';t=f['t']*100
                parts.append(f'<defs><linearGradient id="{gid}" x1="0" y1="1" x2="0" y2="0"><stop offset="{t}%" stop-color="{fill}"/><stop offset="{min(t+.01,100)}%" stop-color="white"/></linearGradient></defs>');fill=f'url(#{gid})'
            parts.append(f'<path d="{s["d"]}" fill="{fill}" stroke="{s["stroke"]}" stroke-width="{s["sw"]}"/>')
        self.svg.append(f'<g transform="translate({x},{y}) scale(.4)">'+''.join(parts)+'</g>')
        if value:self.text(str(value),x+60,y+2,80,24,700)
    def photo(self,x,y,w,h,media):
        self.d.add_obj(self.page,{'id':str(uuid.uuid4()),'name':'バーガー写真（サンプル）','type':'rect','parentId':self.frame,'frameId':self.frame,'fills':[{'fillImage':media,'fillOpacity':1}],'strokes':[],'r1':12,'r2':12,'r3':12,'r4':12,**geom(self.x+x,y,w,h)})
        cid=uuid.uuid4().hex;self.svg.append(f'<defs><clipPath id="{cid}"><rect x="{x}" y="{y}" width="{w}" height="{h}" rx="12"/></clipPath></defs><image href="{PHOTO}" x="{x}" y="{y}" width="{w}" height="{h}" preserveAspectRatio="xMidYMid slice" clip-path="url(#{cid})"/>')
    def logo(self,x,y):
        path=re.search(r'<path d="([^"]+)"',MARK)[1]
        self.d.icon(self.page,{'name':'BurgerStack サービスロゴ','x':x,'y':y,'w':36,'h':30,'icon':{'vb':[120,100],'shapes':[{'part':'サービスロゴ','d':path,'fill':None,'stroke':'#b88f13','sw':5.5,'bbox':[7,6,106,90]}]}},self.x,0,self.frame,self.frame)
        self.svg.append(f'<g transform="translate({x},{y}) scale(.3)"><path d="{path}" fill="none" stroke="#b88f13" stroke-width="5.5"/></g>')
        self.text('Burger Stack',x+44,y,180,20,700)
    def header(self,active='お店'):
        self.logo(self.pad,20)
        if self.mobile:
            self.text('JA / EN',self.w-78,25,62,12,700)
            for x,t in zip([24,102,210],['お店','ハンバーガー','レビュー']):self.text(t,x,75,100,13,700 if active==t else 400)
            self.text('Burger Stackについて',24,111,230,13,700 if active=='about' else 400,underline=True)
            self.rect(0,148,self.w,1,LINE);return 180
        for x,t in zip([420,500,650,785],['お店','ハンバーガー','レビュー','Burger Stackについて']):self.text(t,x,28,200,13,700 if active in [t,'about'] and (t==active or t.startswith('Burger')) else 400)
        self.text('マイページ',1045,28,100,13);self.text('JA / EN',1160,28,90,12)
        self.rect(0,76,self.w,1,LINE);return 122
    def footer(self):
        self.rect(self.pad,self.h-112,self.cw,1,LINE)
        self.text('Burger Stack',self.pad,self.h-86,self.cw,16,700)
        self.text('お気に入りの一口を、積み重ねよう。',self.pad,self.h-54,self.cw,12,color=MUTED)
    def title(self,title,sub,y):
        y=self.text(title,self.pad,y,self.cw,30 if self.mobile else 40,700,line=50)
        return self.text(sub,self.pad,y+10,self.cw,14,color=MUTED)+30
    def filters(self,y,placeholder='名前で探す',sort='評価の高い順'):
        sw=self.cw if self.mobile else self.cw-260
        self.rect(self.pad,y,sw,48,WHITE,8,LINE);self.text(placeholder,self.pad+16,y+11,sw-70,14,color=MUTED)
        self.text('検索',self.pad+sw-52,y+11,42,14,700)
        sy=y+60 if self.mobile else y;sx=self.pad if self.mobile else self.pad+sw+16
        self.rect(sx,sy,244,48,WHITE,8,LINE);self.text(sort+'  ▾',sx+16,sy+11,216,14)
        return sy+76
    def reviewcard(self,x,y,w,media):
        self.rect(x,y,w,590,WHITE,12,LINE)
        self.photo(x,y,w,230,media)
        self.text('クラシックチーズバーガー',x+20,y+252,w-40,21,700,underline=True)
        self.text('サンプルバーガー 下北沢店 →',x+20,y+322,w-40,14,underline=True)
        self.rating(x+20,y+360,4.5)
        self.text('食べた日  2026年9月24日',x+20,y+414,w-40,13,color=MUTED)
        self.text('香ばしいパティと、とろけるチーズ。\nまた食べに行きたくなる一口でした。',x+20,y+444,w-40,14)
        self.text('HAL 1984 →',x+20,y+528,w-40,13,underline=True)
        self.text('レビューを読む →',x+20,y+557,w-40,13,700,underline=True)
    def save(self):
        self.h=max(760,self.bottom+168)
        g=geom(self.x,0,self.w,self.h)
        frame_change=next(c for c in self.d.fb.pending if c.get('type')=='add-obj' and c.get('id')==self.frame)
        frame_change['obj'].update(g)
        self.footer();self.d.fb.flush();OUT.mkdir(parents=True,exist_ok=True)
        svg=f'<svg xmlns="http://www.w3.org/2000/svg" width="{self.w}" height="{self.h}" viewBox="0 0 {self.w} {self.h}"><rect width="100%" height="100%" fill="white"/><g font-family="Noto Sans JP, Hiragino Kaku Gothic ProN, sans-serif">'+''.join(self.svg)+'</g></svg>'
        (OUT/f'{self.key}.svg').write_text(svg)

ABOUT=[('Hello World！','ハンバーガーが好きな、しがないエンジニアです。\n突然ですが、みなさんはどれくらいハンバーガーを食べていますか？\n街を歩けば、あちこちで見かけるハンバーガーのチェーン店。少し調べてみると、個人で営むハンバーガー屋さんも、実は驚くほどたくさんあります。\n身近な食べ物でありながら、お店ごとの個性も楽しめる。それがハンバーガーの魅力だと思っています。'),('ハンバーガーだって、\nもっと語りたい。','でも、ハンバーガー好きとしては、ちょっと物足りないことがあります。\nたとえばラーメンなら、お気に入りのお店を語り合ったり、気になる一杯のために遠くまで出かけたり。そんな楽しみ方が、ハンバーガーではまだ少ないように感じるんです。\nハンバーガーだって、もっと語りたい。もっと食べ歩きたい。\nそして、まだ知らないおいしいお店に出会いたい！\n「世の中に、もっとハンバーガーを！！！」\nそんな気持ちで、このサイトを作りました。'),('いつもの選択肢に、\nハンバーガーを。','目指しているのは、「今日のお昼、何にする？」「夕飯、どこに行こう？」という会話の中で、近所のハンバーガー屋さんが自然と候補に挙がること。\nいつものチェーン店に加えて、店主のこだわりが詰まった一軒も、日々の選択肢になってほしい。そして、お気に入りのお店や一口との出会いを通じて、ハンバーガーを楽しむ仲間が増えてほしいと思っています。'),('「おいしかった！」を、\n積み重ねて。','サイトの名前は、Burger Stack。\nハンバーガーが積み上がっていくイメージから名付けました。\nここに集まる「おいしかった！」が一つずつ積み重なって、いつか月まで届いたらいいな。そんなことを思ったり、思わなかったりしています。')]

def build(d,page,w,media):
    mobile=w<600;mapping=[]
    specs=[('shops','お店を探す',1500 if mobile else 1280),('burgers','ハンバーガー',1500 if mobile else 1280),('reviews','みんなのレビュー',1720 if mobile else 1250),('burger','バーガー詳細',1720 if mobile else 1440),('shop','お店の詳細',1650 if mobile else 1370),('review','レビュー詳細',1660 if mobile else 1320),('profile','マイページ',1690 if mobile else 1300),('empty','マイページ・未投稿',900),('apps','接続済みのアプリ',1020),('about','Burger Stackについて',3750 if mobile else 3100)]
    for ix,(key,name,height) in enumerate(specs):
        b=Board(d,page,name+(' / モバイル' if mobile else ' / PC'),key+('-mobile' if mobile else '-pc'),w,height,ix*(w+160))
        y=b.header('about' if key=='about' else 'レビュー' if key in ['reviews','review'] else 'ハンバーガー' if key in ['burgers','burger'] else 'お店');p=b.pad;cw=b.cw
        if key=='about':
            b.rect(0,y-20,w,350 if mobile else 460,CREAM)
            b.text('BURGER STACK / OUR STORY',p,y+12,cw,12,700,color=MUTED)
            y=b.text('世の中に、もっと\nハンバーガーを！！！',p,y+60,cw,27 if mobile else 60,700,line=46 if mobile else 82)
            b.text('お気に入りの一口を探して、\nハンバーガーの楽しみを広げよう。',p,y+24,cw,15 if mobile else 20)
            y=590 if mobile else 690;tx=p if mobile else 260;tw=cw if mobile else 760
            for heading,body in ABOUT:
                y=b.text(heading,tx,y,tw,23 if mobile else 32,700,line=38 if mobile else 48)+24
                y=b.text(body,tx,y,tw,15 if mobile else 18,line=30 if mobile else 36)+58
            y=b.text('素晴らしき、\nハンバーガーライフを！！！',tx,y,tw,23 if mobile else 34,700)+24
            y=b.text('さあ、あなたもお気に入りの一口を探してみませんか？',tx,y,tw,16)+24
            b.button('お店を探す →',tx,y,180)
            b.text('みんなのレビューを見る →',tx,y+68,tw,14,underline=True)
            if y+240>b.h: raise ValueError(f'About overflow: {y} / {b.h}')
        elif key in ['shops','burgers']:
            y=b.title(('次の一口を、\n見つけよう。' if mobile else '次の一口を、見つけよう。') if key=='shops' else ('気になるバーガーを\n探す' if mobile else '気になるバーガーを探す'),'お店とハンバーガーの出会いを、ここから。',y)
            y=b.filters(y,'店名で探す' if key=='shops' else 'バーガー名で探す','名前順' if key=='shops' else 'ランキング順')
            b.text('6件のお店' if key=='shops' else '6件のハンバーガー',p,y,cw,13,color=MUTED);y+=40
            n=2 if mobile else 6;cols=1 if mobile else 3;gap=24;cardw=(cw-gap*(cols-1))/cols
            for i in range(n):
                x=p+(i%cols)*(cardw+gap);yy=y+(i//cols)*410
                b.photo(x,yy,cardw,240,media)
                b.text(('サンプルバーガー 下北沢店' if key=='shops' else 'クラシックチーズバーガー'),x,yy+254,cardw,19,700,underline=True)
                b.rating(x,yy+322,4.5)
                b.text('12件のレビュー →',x+140,yy+335,cardw-140,12,underline=True)
        elif key=='reviews':
            y=b.title(name,'みんなの「おいしかった！」を見てみよう。',y);y=b.filters(y,'レビューのコメントで探す','新しい順')
            cardw=cw if mobile else (cw-24)/2
            for i in range(2):b.reviewcard(p if mobile else p+i*(cardw+24),y+i*614 if mobile else y,cardw,media)
        elif key in ['burger','shop','review']:
            b.text('お店 → ハンバーガー → レビュー',p,y,cw,12,color=MUTED,underline=True);y+=40
            pw=cw if mobile else 620;ph=250 if mobile else 410;b.photo(p,y,pw,ph,media)
            tx=p if mobile else p+660;ty=y+ph+24 if mobile else y+16;tw=cw if mobile else cw-660
            ty=b.text('サンプルバーガー\n下北沢店' if key=='shop' else 'クラシック\nチーズバーガー',tx,ty,tw,28 if mobile else 34,700)+12
            ty=b.text('東京都・下北沢' if key=='shop' else 'サンプルバーガー 下北沢店 →',tx,ty,tw,14,underline=key!='shop')+18
            b.rating(tx,ty,4.5);ty+=60;b.text('12件のレビュー',tx,ty,tw,14,color=MUTED);b.button('レビューを書く',tx,ty+40,180)
            yy=max(y+ph+56,ty+124)
            b.text('HAL 1984のレビュー' if key=='review' else 'みんなのレビュー',p,yy,cw,25,700);yy+=62
            b.rect(p,yy,cw,370,CREAM,12)
            b.text('HAL 1984 →',p+24,yy+24,cw-48,16,700,underline=True);b.rating(p+24,yy+68,4.5)
            b.text('食べた日  2026年9月24日',p+24,yy+128,cw-48,14)
            end=b.text('香ばしいパティと、とろけるチーズ。\nボリュームたっぷりで、最後の一口まで楽しめました。',p+24,yy+170,cw-48,16)
            b.text('投稿日  2026年9月25日',p+24,end+16,cw-48,12,color=MUTED)
            b.text('レビュー詳細 →',p+24,end+48,cw-48,14,700,underline=True)
        elif key in ['profile','empty']:
            y=b.title('HAL 1984','ハンバーガーが好きです。お気に入りの一口を記録中。',y)
            b.button('プロフィールを編集',p,y,190,False);y+=84
            b.text('食べたバーガー',p,y,cw,25,700);y+=58
            if key=='empty':
                b.rect(p,y,cw,280,CREAM,12);b.rating(p+24,y+28,0)
                b.text('何も食べていません',p+24,y+96,cw-48,23,700)
                b.text('最初の一口を記録してみよう。',p+24,y+144,cw-48,14,color=MUTED)
                b.button('お店を探す →',p+24,y+202,180)
            else:
                cardw=cw if mobile else (cw-24)/2
                for i in range(2):b.reviewcard(p if mobile else p+i*(cardw+24),y+i*614 if mobile else y,cardw,media)
        elif key=='apps':
            y=b.title(name,'Burger Stackと連携しているアプリを管理できます。',y)
            b.rect(p,y,cw,440 if mobile else 350,WHITE,12,LINE)
            b.text('ChatGPT',p+24,y+24,cw-48,26,700)
            yy=b.text('許可していること',p+24,y+84,cw-48,16,700)+16
            yy=b.text('お店・ハンバーガー・レビュー・プロフィールの読み取り\nレビューの投稿と編集',p+24,yy,cw-48,15)+24
            yy=b.text('接続日  2026年9月24日',p+24,yy,cw-48,13,color=MUTED)+24
            b.button('接続を解除する',p+24,yy,176,False)
        b.save();mapping.append({'key':b.key,'page':page,'frame':b.frame,'width':w,'height':b.h})
    return mapping

def main():
    a=argparse.ArgumentParser();a.add_argument('--file-id',required=True);a.add_argument('--email',required=True);a.add_argument('--password-file',required=True);args=a.parse_args()
    api=Penpot('http://localhost:9001');api.call('login-with-password',{'email':args.email,'password':Path(args.password_file).read_text().strip()})
    uploaded=json.loads(multipart(api,'upload-file-media-object',{'file-id':args.file_id,'name':'デザイン用サンプル写真','is-local':'true'},'sample-burger.jpg',(ASSETS/'sample-burger.jpg').read_bytes()))
    media={k:uploaded[k] for k in ['id','width','height','mtype']};print('Photo uploaded',flush=True)
    fb=BufferedFileBuilder(api,args.file_id);d=Design(fb,{}, {},args.file_id,font=('gfont-noto-sans-jp','Noto Sans JP'),weights=(400,700),baseline=.8,snap=True)
    old=api.call('get-file',{'id':args.file_id})['data']
    for pid,p in old.get('pagesIndex',{}).items():
        if p.get('name','').startswith('2026-09 Burger Stack / '):fb.add({'type':'del-page','id':pid})
    fb.flush()
    mapping=[]
    for w,label in [(1280,'PC'),(375,'モバイル')]:
        page=str(uuid.uuid4());fb.add({'type':'add-page','id':page,'name':'2026-09 Burger Stack / '+label});fb.flush();mapping+=build(d,page,w,media);print(label+' boards added',flush=True)
    (ROOT/'design/refresh/boards.json').write_text(json.dumps({'fileId':args.file_id,'boards':mapping},ensure_ascii=False,indent=2)+'\n')
    api.call('rename-file',{'id':args.file_id,'name':'Burger Stack — 全体デザイン 2026-09'})
    req=urllib.request.Request(api.base+'/api/rpc/command/export-binfile',data=json.dumps({'fileId':args.file_id,'version':3,'includeLibraries':False,'embedAssets':True}).encode(),headers={'Content-Type':'application/json','Accept':'application/json'})
    res=api.opener.open(req).read().decode();(Path(args.password_file).parent/'export-result.txt').write_text(res)
    urls=re.findall(r'https?[^"\s]+',res)
    if not urls:raise RuntimeError('Penpot export did not return a download URL')
    destination=ROOT/'design/files/burgerstack-refresh-2026-09.penpot'
    destination.write_bytes(api.opener.open(urls[-1]).read())
    print('Export saved:',destination,flush=True)
if __name__=='__main__':main()
