"""Build the Windows COFF .rsrc from assets/app.ico without external tools."""
from pathlib import Path
import struct
ROOT=Path(__file__).resolve().parents[1]
ico=(ROOT/'assets/app.ico').read_bytes();count=struct.unpack_from('<H',ico,4)[0]
resources={};group=struct.pack('<HHH',0,1,count)
for i in range(count):
    w,h,c,r,planes,bits,n,off=struct.unpack_from('<BBBBHHII',ico,6+i*16)
    resources[(3,i+1,1033)]=ico[off:off+n]
    group+=struct.pack('<BBBBHHIH',w,h,c,r,planes,bits,n,i+1)
resources[(14,1,1033)]=group
manifest=b'''<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<assembly xmlns="urn:schemas-microsoft-com:asm.v1" manifestVersion="1.0">
<assemblyIdentity version="2.2.0.0" processorArchitecture="amd64" name="ApplyKit.Desktop" type="win32"/>
<description>ApplyKit offline document preparation</description>
<trustInfo xmlns="urn:schemas-microsoft-com:asm.v3"><security><requestedPrivileges><requestedExecutionLevel level="asInvoker" uiAccess="false"/></requestedPrivileges></security></trustInfo>
<compatibility xmlns="urn:schemas-microsoft-com:compatibility.v1"><application><supportedOS Id="{8e0f7a12-bfb3-4fe8-b9a5-48fd50a15a9a}"/></application></compatibility>
<application xmlns="urn:schemas-microsoft-com:asm.v3"><windowsSettings><dpiAware xmlns="http://schemas.microsoft.com/SMI/2005/WindowsSettings">true/pm</dpiAware><longPathAware xmlns="http://schemas.microsoft.com/SMI/2016/WindowsSettings">true</longPathAware></windowsSettings></application>
</assembly>'''
resources[(24,1,1033)]=manifest

def u16(s): return (s+'\0').encode('utf-16le')
def pad(b): return b+b'\0'*((-len(b))%4)
def block(key,value=b'',children=(),typ=0,units=None):
    b=pad(struct.pack('<HHH',0,len(value) if units is None else units,typ)+u16(key));b+=value
    if children:b=pad(b)+b''.join(pad(x) for x in children)
    return struct.pack('<H',len(b))+b[2:]
strings={
 'CompanyName':'ApplyKit','FileDescription':'投递材料助手 · PDF / 图片裁剪、转换与压缩','FileVersion':'2.2.0.0',
 'InternalName':'ApplyKit','OriginalFilename':'ApplyKit.exe','ProductName':'ApplyKit 投递材料助手','ProductVersion':'2.2.0',
 'LegalCopyright':'ApplyKit 2026 - MIT licensed source included'
}
strtable=block('040904B0',children=[block(k,u16(v),typ=1,units=len(u16(v))//2) for k,v in strings.items()],typ=1)
stringinfo=block('StringFileInfo',children=[strtable],typ=1);varinfo=block('VarFileInfo',children=[block('Translation',struct.pack('<HH',1033,1200))],typ=1)
fixed=struct.pack('<13I',0xFEEF04BD,0x10000,0x20002,0,0x20002,0,0x3f,0,0x40004,1,0,0,0)
resources[(16,1,1033)]=block('VS_VERSION_INFO',fixed,[stringinfo,varinfo])
buf=bytearray();data_entries=[]
def reserve(n):off=len(buf);buf.extend(b'\0'*n);return off
def align():buf.extend(b'\0'*((-len(buf))%4))
def directory(tree,depth=0):
    ids=sorted({k[depth] for k in tree});off=reserve(16+8*len(ids));struct.pack_into('<IIHHHH',buf,off,0,0,0,0,0,len(ids))
    for i,ident in enumerate(ids):
        subset={k:v for k,v in tree.items() if k[depth]==ident}
        if depth<2:child=directory(subset,depth+1)|0x80000000
        else:align();child=reserve(16);data_entries.append((child,next(iter(subset.values()))))
        struct.pack_into('<II',buf,off+16+i*8,ident,child)
    return off
directory(resources);relocs=[]
for entry,data in data_entries:
    align();off=len(buf);buf.extend(data);struct.pack_into('<IIII',buf,entry,off,len(data),0,0);relocs.append(struct.pack('<IIH',entry,0,3))
align();raw=bytes(buf);reloc=b''.join(relocs);raw_off=60;reloc_off=raw_off+len(raw);sym_off=reloc_off+len(reloc)
header=struct.pack('<HHIIIHH',0x8664,1,0,sym_off,1,0,0);section=struct.pack('<8sIIIIIIHHI',b'.rsrc\0\0\0',0,0,len(raw),raw_off,reloc_off,0,len(relocs),0,0x40300040);symbol=struct.pack('<8sIhHBB',b'.rsrc\0\0\0',0,1,0,3,0)
(ROOT/'resource_windows_amd64.syso').write_bytes(header+section+raw+reloc+symbol+struct.pack('<I',4))
print('Windows resources:',len(raw),'bytes;',len(resources),'resources;',count,'icons')
