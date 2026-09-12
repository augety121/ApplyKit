# Release checklist

- [ ] `const version`、CHANGELOG 和 tag 一致。
- [ ] `go test -count=1 ./...` 通过。
- [ ] `go vet ./...` 通过。
- [ ] `build.ps1` 生成 Windows GUI x64 EXE。
- [ ] `tools/windows_smoke.ps1` 在真实 Windows 通过。
- [ ] 200 KB / 500 KB / 1 MB / 2 MB 以及一个小数 MB 自定义限制至少抽测一次。
- [ ] PDF 自动裁剪、手动裁剪、空白页保护抽测。
- [ ] 图片裁剪、旋转、最长边抽测。
- [ ] 1024×768、1366×768 与 1536×1024 窗口检查队列、结果、设置、底部操作栏无覆盖。
- [ ] EXE 图标、标题栏 favicon 与 README 图标一致。
- [ ] Release ZIP 只包含软件、文档和合成样本，不包含个人材料。
- [ ] 如维护者显式选择生成校验和，再使用 `-WriteChecksums`；默认不生成。
