# 合成测试样本 / Synthetic fixtures

这些样本仅用于软件测试，不包含真实个人材料，也不是真实证件。

- `crop-demo.pdf`：三页同尺寸 PDF。第 1 页有大白边；第 2 页边缘有浅色 `KEEP EDGE MARK`，自动裁剪应保留它；第 3 页故意全白，自动模式应保留完整页面。
- `text-demo.png`：合成文字排版，用于检查压缩后的细字可读性。
- `transparent-demo.png`：合成透明图，用于检查白底合成与图片转 PDF。

自动裁剪默认：阈值 250，安全边距 2mm。开启“统一内容边界”时，前两页的内容区域取并集；全白页依然保留整页。手动裁剪尊重用户明确选择，不会代替用户判断应保留哪些内容。

重建 PDF 样本：`python tools/make_test_samples.py`，仅开发时需要 ReportLab；最终软件不需要 Python。
