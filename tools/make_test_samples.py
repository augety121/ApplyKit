"""Regenerate synthetic crop fixtures. Development only; needs reportlab.
No personal documents, institutional marks or external font files are used.
"""
from pathlib import Path
from reportlab.pdfgen import canvas
from reportlab.lib.colors import HexColor, Color
ROOT=Path(__file__).resolve().parents[1]
path=ROOT/'samples/crop-demo.pdf'
path.parent.mkdir(parents=True,exist_ok=True)
c=canvas.Canvas(str(path),pagesize=(600,840),pageCompression=1,invariant=1)
c.setTitle('ApplyKit synthetic crop fixture - 3 pages')
c.setAuthor('ApplyKit test fixtures')
c.setSubject('Synthetic test material; not an application document')
def body(left=130, bottom=190, width=350, height=440, page=1):
 c.setFillColor(HexColor('#143A50'));c.roundRect(left,bottom,width,height,12,fill=1,stroke=0)
 c.setFillColor(HexColor('#C2ECE0'));c.setFont('Helvetica-Bold',13);c.drawString(left+22,bottom+height-38,'APPLYKIT / CROP TEST')
 c.setFillColor(HexColor('#FFFFFF'));c.setFont('Helvetica-Bold',22);c.drawString(left+22,bottom+height-82,'Keep this content.')
 c.setFont('Helvetica',11)
 for i,text in enumerate(['Synthetic document. No personal data.', 'Use the white margins to test cropping.', 'The export must keep all visible content.', 'Inspect small text before uploading.']):
  c.drawString(left+22,bottom+height-117-i*24,text)
 c.setFillColor(HexColor('#86C9BA'))
 for i in range(5):c.roundRect(left+22,bottom+80+i*22,width-52-i*20,5,2,fill=1,stroke=0)
 c.setFillColor(HexColor('#FFFFFF'));c.setFont('Helvetica',8);c.drawString(left+22,bottom+35,f'Page {page} | TEST-ONLY-2026 | 0123456789')
body();c.showPage()
body(110,160,360,470,page=2)
c.setFillColor(Color(1,.83,.83));c.setFont('Helvetica-Bold',8);c.drawString(478,35,'KEEP EDGE MARK')
c.showPage()
# Intentionally blank: no page number, text, line or background rectangle.
c.showPage();c.save()
print(path)
