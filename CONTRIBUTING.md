# Contributing

感谢参与 ApplyKit。提交前请先阅读 `README.md`、`docs/ARCHITECTURE.md` 与 `docs/TESTING.md`。

## 设计约束

- 文件体积和最终成功判断必须由 Go 后端按真实字节完成，不能只在界面层判断。
- 不覆盖输入文件，不用“未编辑原件”冒充编辑成功。
- PDF 手动裁剪和图片编辑方案必须使用归一化坐标，并用源文件 SHA-256 防止材料变更后继续套用旧方案。
- Windows 专属 PDF 渲染必须由真实 Windows 测试证明；Linux mock 只用于算法测试。
- UI 需保持 queue / results / settings / export dock 四个布局区域互不覆盖，并兼顾 1246×877 及更小窗口。
- 不在仓库提交真实简历、证件、成绩单、密码或招聘网站材料。

## 提交前

```bash
go test -count=1 ./...
go vet ./...
```

Windows 上还应运行：

```powershell
powershell.exe -NoLogo -NoProfile -STA -ExecutionPolicy RemoteSigned -File ./tools/windows_smoke.ps1
```

修改品牌图标后运行：

```bash
python tools/make_brand.py
python tools/make_resources.py
```
