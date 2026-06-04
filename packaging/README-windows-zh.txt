AI 状态红绿灯安装说明 - Windows

适用场景：

这份安装器适用于已经预先配置好的 AI 状态红绿灯。

安装步骤：

1. 把 AI 状态红绿灯插到电脑上。

2. 解压安装包。

3. 双击运行安装器：

   CodexTrafficLightInstaller-windows-x86_64.exe

   如果你的电脑是 32 位 Windows，请使用：

   CodexTrafficLightInstaller-windows-x86_32.exe

4. 如果 Windows 安全提示拦截，确认这是你信任的安装器后，选择“仍要运行”。

5. 打开 Codex，输入：

   /hooks

6. 在 hooks 页面中信任新的 AI 状态红绿灯 hook。

完成后即可使用。

灯光含义：

- Codex 开始工作：黄灯常亮
- Codex 正常结束：绿灯闪烁
- Codex 请求权限或工具执行失败：红灯闪烁
