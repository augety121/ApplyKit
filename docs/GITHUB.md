# 上传 GitHub 与发布软件

## 放哪些文件

解压源码包，把里面包含 `README.md`、`go.mod`、`assets/`、`.github/` 的这一层作为仓库根目录。不要再套一层无意义的压缩包文件夹。Windows 文件管理器可能不明显显示以点开头的目录；`.github` 中的工作流也要提交。

建议仓库名 `ApplyKit`，描述可使用：

> 离线 Windows 投递材料工具：PDF 转图片、白边裁剪、图片压缩与图片合并 PDF，支持严格上传大小限制。

源码已包含 README（中文/英文）、MIT LICENSE、忽略规则、构建脚本、测试、Issue 模板和发布流程。不要把真实简历、证件、成绩单、密码或处理结果加入仓库；这些不属于代码。

## 首次上传

在 GitHub 创建一个空仓库。以下地址是需要替换的占位符，不表示已经为你创建了仓库。

```bash
git init
git add .
git commit -m "feat: ApplyKit 1.1 with PDF crop workbench"
git branch -M main
git remote add origin https://github.com/YOUR_GITHUB_USERNAME/ApplyKit.git
git push -u origin main
```

使用网页上传时，也应上传解压后的源码文件，而不是只上传 Source.zip。Git 可以更可靠地保留目录结构。

## 首次发布 1.1.0

先在 Windows 运行软件包中的 `运行自检.cmd`，完成交互检查，再确认该仓库的 `Build and test` Actions 记录通过。随后推送标签：

```bash
git tag v1.1.0
git push origin v1.1.0
```

标签工作流会在 Windows 上执行测试、构建、原生冒烟测试、打包，再用 GitHub CLI 创建 Release 并附加 Windows ZIP。需要仓库允许 Actions；工作流中已声明 `contents: write` 权限。组织策略可以覆盖该权限。失败时应查看运行日志，不能把失败构建标成正式版。

也可以手动创建 Release，将已经生成的软件 ZIP 上传为附件。普通使用者应下载这个附件，而不是 GitHub 自动生成的 Source code ZIP。

当前工作流不覆盖已有 Release 或同名附件。不要重复删除/重建标签来掩盖失败；检查失败原因并用新补丁版本发布。

## 后续版本

更新 `main.go` 中的版本号、Windows 资源生成脚本中的版本号并重新生成 `.syso`，更新 CHANGELOG，运行测试。新版本标签必须与源码版本一致。普通用户不需要这些开发步骤。

这次交付只提供文件，没有自动创建仓库、提交、推送或修改你的 GitHub 内容。
