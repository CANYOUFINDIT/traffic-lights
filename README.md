# AI 状态红绿灯

这是一个用实体红黄绿灯显示 Codex 工作状态的小项目。

灯光含义：

- Codex 开始工作：黄灯常亮
- Codex 正常结束：绿灯闪烁
- Codex 请求权限或工具执行失败：红灯闪烁

## 用户安装

如果你拿到的是已经预先配置好的 AI 状态红绿灯，不需要安装 Python 或 Go，也不需要重新刷固件。

到 GitHub Releases 下载对应系统的安装包：

- macOS Apple Silicon：`CodexTrafficLightInstaller-macos-arm64.zip`
- Windows 64 位：`CodexTrafficLightInstaller-windows-amd64.zip`
- Windows 32 位：`CodexTrafficLightInstaller-windows-386.zip`

解压后按压缩包里的中文说明运行安装器，然后在 Codex 中输入 `/hooks` 并信任新的 hook。

## 目录结构

```text
cmd/codex-traffic-light/   # 安装器和 Codex hook 工具源码
firmware/main.py           # AI 状态红绿灯固件
firmware/*.bin             # ESP32-C3 MicroPython 固件
packaging/                 # 安装包内的中文说明
scripts/                   # 构建安装包、刷机、上传固件脚本
```

## 开发环境

构建安装器需要 Go。

```bash
go env -w GOPROXY=https://goproxy.io,direct
go mod tidy
```

构建 macOS 和 Windows 安装包：

```bash
./scripts/build-installers.sh
```

构建产物会生成到 `dist/`，其中 zip 文件用于发布到 GitHub Release。

## 本机安装与测试

开发时可以直接构建并安装 hook：

```bash
go build -o codex-traffic-light ./cmd/codex-traffic-light
./codex-traffic-light install
```

检查串口和 hook 配置：

```bash
./codex-traffic-light doctor
./codex-traffic-light ports
```

手动测试灯光：

```bash
./codex-traffic-light send yellow
./codex-traffic-light send blink_green
./codex-traffic-light send blink_red
./codex-traffic-light send off
```

查看脱敏 hook 日志：

```bash
./codex-traffic-light logs 20
./codex-traffic-light clear-logs
```

## 固件上传

如果 AI 状态红绿灯已经有 MicroPython，只需要上传 `firmware/main.py`：

```bash
python3 -m pip config set global.index-url https://pypi.tuna.tsinghua.edu.cn/simple
python3 -m pip config set global.timeout 120
python3 -m pip install --upgrade esptool mpremote pyserial
```

查找串口：

```bash
python3 -m serial.tools.list_ports -v
```

上传固件代码：

```bash
./scripts/upload-firmware.sh /dev/cu.usbmodemXXXX
```

Windows 端口示例：

```powershell
.\scripts\upload-firmware.ps1 -Port COM3
```

如果需要从零刷 MicroPython：

```bash
./scripts/flash-micropython.sh /dev/cu.usbmodemXXXX
```

Windows 示例：

```powershell
.\scripts\flash-micropython.ps1 -Port COM3
```

## Codex hook 映射

安装器会写入 `~/.codex/hooks.json`，映射如下：

- `UserPromptSubmit` -> `yellow`
- `PermissionRequest` -> `blink_red`
- `PostToolUse` 失败时 -> `blink_red`
- `Stop` -> `blink_green`
