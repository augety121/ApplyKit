# 上传 GitHub / 发布版本

## 仓库根目录

解压源码包后，**把包含 `README.md`、`go.mod`、`web/`、`assets/`、`.github/` 的 `ApplyKit` 文件夹内容作为仓库根目录**，不要再额外套一层压缩包。

建议：

```bash
git init
git add .
git commit -m "release: ApplyKit 2.2.0"
git branch -M main
git remote add origin <your-repository-url>
git push -u origin main
```

不要提交真实简历、证件、成绩单或招聘网站材料。本仓库 `samples/` 只包含合成测试文件。

## GitHub Actions

- `.github/workflows/ci.yml`：Ubuntu + Windows 单元测试、静态检查、Windows 构建、真机组件自检和 ZIP Artifact。
- `.github/workflows/release.yml`：推送 `v*` tag 时，在 Windows Runner 重新验证并发布 Release ZIP。

## 发布 2.2.0

确认 Actions 绿色后：

```bash
git tag v2.2.0
git push origin v2.2.0
```

Release workflow 会校验 tag 与 `main.go` 中版本号一致，再创建 GitHub Release。不要把开发环境里的 Linux 验证当作 `Windows.Data.Pdf` 真机验收；正式 Release 由 Windows Runner 执行 `tools/windows_smoke.ps1`。

## 本地重新构建

```powershell
./build.ps1
./tools/package.ps1
```

输出在 `dist/`。
