# Contributing

使用当前受支持的 Go 版本。提交前执行 `go test -count=1 ./...`、`go vet ./...`，并在 Windows PowerShell 5.1 中执行 `tools/windows_smoke.ps1`。

图像/预算/文件保护逻辑在 Go 中；Windows 原生渲染和 WPF UI 在 `assets/`。不要把字节校验只放在界面层。更改裁剪坐标时需要同时验证低分辨率预览和高分辨率导出。

不要提交个人证件、真实求职材料、密码、输出报告、缓存或未脱敏日志。用 `samples/` 的人工测试材料重现问题。不要把 `.syso` 资源丢掉；普通用户构建不应被迫安装图标资源生成器。

发布前更新 `main.go` 的版本号、`tools/make_resources.py` 的版本字段、重建资源，更新 CHANGELOG 并完成 `docs/RELEASE_CHECKLIST.md`。人工资源重建才需要 Python/Pillow。

PR 描述应说明改动、复现步骤、风险和实际执行的测试。不要把模拟渲染测试写成真实 Windows PDF 渲染验收。
