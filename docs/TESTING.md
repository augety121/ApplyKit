# 测试

```powershell
go test -count=1 ./...
go vet ./...
node --check web/app.js
node --check web/ui.js
node --check web/editor.js
node --test tests/geometry.test.mjs
./build.ps1 -SkipTests
powershell.exe -NoLogo -NoProfile -STA -ExecutionPolicy RemoteSigned -File ./tools/windows_smoke.ps1
```

Go 测试包含算法与本地 HTTP API；Node 内置测试运行器验证 PDF 单位、旋转尺寸和复合变换。Windows 自检只使用合成材料，生成 `out/windows-smoke-*/acceptance.json`。自检不依赖 Python 或 Node.js；便携包用 `运行自检.cmd` 调用同一个脚本。

界面修改需真实浏览器检查：首次启动、导入、清空、预览翻页、裁剪模式、边距、图片编辑、大小验证、导出结果、忙碌与失败状态；至少检查 1366×768、1536×1024 和深色主题。文件名、表格列和操作栏不能产生水平溢出。手动浏览器验收的范围写入 `docs/ACCEPTANCE.md`。
