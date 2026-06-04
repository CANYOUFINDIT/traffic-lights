 AI 状态红绿灯安装说明 - macOS Apple Silicon

适用场景：

这份安装器适用于已经预先配置好的 AI 状态红绿灯。

安装步骤：

1. 把 AI 状态红绿灯插到电脑上。

2. 解压安装包后，打开“终端”，进入安装器所在的文件夹。

3. 给安装器加执行权限：

   chmod +x ./CodexTrafficLightInstaller-macos-arm64

4. 如果 macOS 提示“无法打开，因为它来自身份不明的开发者”，或提示文件
   来自互联网，可以先移除隔离标记：

   xattr -d com.apple.quarantine ./CodexTrafficLightInstaller-macos-arm64

5. 运行安装器：

   ./CodexTrafficLightInstaller-macos-arm64

6. 打开 Codex，输入：

   /hooks

7. 在 hooks 页面中信任新的 AI 状态红绿灯 hook。

完成后即可使用。

灯光含义：

- Codex 开始工作：黄灯常亮
- Codex 正常结束：绿灯闪烁
- Codex 请求权限或工具执行失败：红灯闪烁
