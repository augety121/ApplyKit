from pathlib import Path
from PIL import Image, ImageDraw, ImageFilter
import math
ROOT=Path(__file__).resolve().parents[1]
assets=ROOT/'assets'; web=ROOT/'web'; docs=ROOT/'docs'
assets.mkdir(exist_ok=True); web.mkdir(exist_ok=True); docs.mkdir(exist_ok=True)
S=1024
im=Image.new('RGBA',(S,S),(0,0,0,0))
# soft shadow
shadow=Image.new('RGBA',(S,S),(0,0,0,0)); sd=ImageDraw.Draw(shadow)
sd.rounded_rectangle((78,90,946,958), radius=225, fill=(1,40,118,105))
shadow=shadow.filter(ImageFilter.GaussianBlur(40)); im.alpha_composite(shadow)
# gradient rounded square mask
mask=Image.new('L',(S,S),0); md=ImageDraw.Draw(mask); md.rounded_rectangle((64,64,960,960),radius=224,fill=255)
grad=Image.new('RGBA',(S,S),(0,0,0,0)); gp=grad.load()
for y in range(S):
    for x in range(S):
        # diagonal azure -> cobalt -> deep blue
        t=max(0,min(1,(x*.48+y*.52-90)/(S*1.05)))
        # slight bright top-left radial lift
        r=int(42*(1-max(0,min(1,math.hypot(x-240,y-190)/620))))
        c0=(51+r,149+r//2,255)
        c1=(16,70,216)
        gp[x,y]=tuple(int(c0[i]*(1-t)+c1[i]*t) for i in range(3))+(255,)
grad.putalpha(mask); im.alpha_composite(grad)
# subtle highlight border
hl=Image.new('RGBA',(S,S),(0,0,0,0)); hd=ImageDraw.Draw(hl)
hd.rounded_rectangle((68,68,956,956),radius=220,outline=(255,255,255,65),width=7)
im.alpha_composite(hl)
d=ImageDraw.Draw(im)
# Stylized ApplyKit A / document mark. Drawn as clean polygons with broad optical weight.
white=(255,255,255,255); soft=(220,236,255,255)
# left leg
left=[(250,744),(410,294),(470,294),(518,425),(402,744)]
d.polygon(left, fill=white)
# right leg / folded-document stroke
right=[(470,294),(596,294),(772,744),(632,744),(586,610),(449,610),(489,500),(548,500)]
d.polygon(right, fill=white)
# inner two document lines on right half
for y,w in [(660,134),(706,175)]:
    d.rounded_rectangle((472,y,472+w,y+24),radius=12,fill=soft)
# tiny page fold notch at top right, echoes a document corner
d.polygon([(596,294),(673,371),(617,371)],fill=(186,220,255,255))
# bottom shine
shine=Image.new('RGBA',(S,S),(0,0,0,0)); sh=ImageDraw.Draw(shine)
sh.ellipse((190,720,840,1010),fill=(255,255,255,24)); shine=shine.filter(ImageFilter.GaussianBlur(30)); im.alpha_composite(shine)
# save PNG and ICO
png=assets/'app-icon-1024.png'; im.save(png)
im.save(assets/'app.ico', format='ICO', sizes=[(16,16),(24,24),(32,32),(40,40),(48,48),(64,64),(128,128),(256,256)])
im.resize((512,512),Image.Resampling.LANCZOS).save(docs/'app-icon-preview.png')
# matching vector favicon/logo
svg='''<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 128 128" role="img" aria-label="ApplyKit logo">
<defs><linearGradient id="g" x1="0" y1="0" x2="1" y2="1"><stop stop-color="#3196ff"/><stop offset="1" stop-color="#1047d8"/></linearGradient><filter id="s" x="-20%" y="-20%" width="140%" height="140%"><feDropShadow dx="0" dy="4" stdDeviation="4" flood-color="#073a9a" flood-opacity=".22"/></filter></defs>
<rect x="8" y="8" width="112" height="112" rx="28" fill="url(#g)" filter="url(#s)"/><rect x="8.7" y="8.7" width="110.6" height="110.6" rx="27.3" fill="none" stroke="#fff" stroke-opacity=".22"/>
<path fill="#fff" d="M31.3 93 51.3 36.8h7.5l6 16.4L50.4 93H31.3Zm27.5-56.2h15.8L96.7 93H79.2l-6-17.2H56.7L62 61.9h6.8l-10-25.1Z"/>
<path fill="#dcecff" d="M59 82.2h16.5a1.6 1.6 0 0 1 0 3.2H59a1.6 1.6 0 1 1 0-3.2Zm0 6h21.5a1.6 1.6 0 0 1 0 3.2H59a1.6 1.6 0 1 1 0-3.2Z"/>
<path fill="#badcff" d="M74.6 36.8 84.2 46h-7Z"/>
</svg>'''
(web/'brand.svg').write_text(svg,encoding='utf-8')
(assets/'brand.svg').write_text(svg,encoding='utf-8')
print('brand assets generated')
