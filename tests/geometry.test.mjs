import test from 'node:test';
import assert from 'node:assert/strict';
import { materialSize, drawTransformed } from '../web/geometry.mjs';

test('PDF point dimensions agree with a 200 DPI render, independent of preview resolution', () => {
  assert.deepEqual(materialSize({kind:'pdf'}, {pageWidth:600,pageHeight:840,sourceWidth:917,sourceHeight:1283},200), [1667,2333]);
  assert.deepEqual(materialSize({kind:'pdf'}, {pageWidth:600,pageHeight:840,sourceWidth:917,sourceHeight:1283},300), [2500,3500]);
});

test('rotated image dimensions use full source pixels, not thumbnail pixels', () => {
  const meta={sourceWidth:2400,sourceHeight:1600,width:1600,height:1067};
  assert.deepEqual(materialSize({kind:'image'},meta,200,90),[1600,2400]);
  assert.deepEqual(materialSize({kind:'image'},meta,200,180),[2400,1600]);
});

test('combined rotation and display-axis flips place the asymmetric source corners correctly', () => {
  // A minimal Canvas affine context projects the drawn corners; expected positions
  // are derived from the public image-edit contract, not from operation call order.
  const corners=[];let m=[1,0,0,1,0,0];
  const multiply=(n)=>{const [a,b,c,d,e,f]=m,[g,h,i,j,k,l]=n;m=[a*g+c*h,b*g+d*h,a*i+c*j,b*i+d*j,a*k+c*l+e,b*k+d*l+f]};
  const context={save(){},restore(){},translate(x,y){multiply([1,0,0,1,x,y])},scale(x,y){multiply([x,0,0,y,0,0])},rotate(t){multiply([Math.cos(t),Math.sin(t),-Math.sin(t),Math.cos(t),0,0])},drawImage(image,x,y){for(const [px,py] of [[x,y],[x+image.width,y],[x,y+image.height]]) corners.push([Math.round(m[0]*px+m[2]*py+m[4])||0,Math.round(m[1]*px+m[3]*py+m[5])||0])}};
  drawTransformed(context,{width:4,height:2},{rotation:90,flipH:true},2,4);
  assert.deepEqual(corners,[[0,0],[0,4],[2,0]]);
});
