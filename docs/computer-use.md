# Computer Use 配方

`remote-shell` 的标准输入、标准输出、标准错误支持二进制数据流，因此截图、键鼠输入均可通过远端 Shell 命令完成，无需在远端安装额外代理。前提是远端存在活跃解锁的桌面会话。

所有命令必须用 `-conn 名称` 指定连接（示例按 `linux`/`mac`/`win` 命名，请替换为你自己的配置名）。

## 前提条件

| 平台 | 必需条件 |
|------|----------|
| Linux | X11 或 Wayland 会话已登录；至少安装 `xwd`、`maim` 或 ImageMagick `import` 之一 |
| macOS | 用户已登录 Aqua 桌面；已授权 Screen Recording 和 Accessibility（首次运行弹窗） |
| Windows | OpenSSH Server 已启用（`Add-WindowsCapability OpenSSH.Server~~~~0.0.1.0`）；已登录桌面会话 |

## 检测远端环境

使用 `remote-shell-info -json` 检查返回的 JSON：
- `os` 字段：远端平台（`Linux`、`Darwin`、`Windows_NT`），决定使用哪套命令
- `default_shell` 字段：远端执行命令所用的默认 Shell——POSIX 下为登录 Shell（`/bin/bash`、`/bin/zsh` 等），Windows 下为 `cmd` 或 `powershell`，决定 `-c` 中的命令语法

TOML 配置中可显式设置 `shell = "cmd"` 或 `shell = "powershell"`，跳过自动探测。

## 截图

### Linux (X11)

```sh
# 自动探测 DISPLAY，优先 xwd，回退 maim/import
remote-shell -conn linux -c 'DISPLAY="${DISPLAY:-:0}"; XAUTHORITY="${XAUTHORITY:-$HOME/.Xauthority}"; command -v xwd >/dev/null 2>&1 && { xwd -root -silent | convert xwd:- png:-; } || command -v maim >/dev/null 2>&1 && maim --format png /dev/stdout || import -window root png:-' > screen.png
```

### macOS

```sh
# screencapture 是内置工具，-x 无音效，-t png 指定格式
remote-shell -conn mac -c 'screencapture -x -t png /tmp/rs-screenshot.png && cat /tmp/rs-screenshot.png' > screen.png
```

### Windows

根据 `default_shell` 选择对应的命令：

**PowerShell（`default_shell` 为 `powershell`）：**

```sh
# PowerShell CopyFromScreen，Session 0 下会失败（需要活跃桌面）
remote-shell -conn win -c 'powershell -NoProfile -ExecutionPolicy Bypass -Command "Add-Type -AssemblyName System.Drawing,System.Windows.Forms; $b=New-Object Drawing.Bitmap([Windows.Forms.Screen]::PrimaryScreen.Bounds.Width,[Windows.Forms.Screen]::PrimaryScreen.Bounds.Height); [Drawing.Graphics]::FromImage($b).CopyFromScreen(0,0,0,0,$b.Size); $f=[IO.Path]::Combine([IO.Path]::GetTempPath(),[IO.Path]::GetRandomFileName()+\".png\"); $b.Save($f,[Drawing.Imaging.ImageFormat]::Png); cat $f; Remove-Item $f -ErrorAction SilentlyContinue"' > screen.png
```

**cmd.exe（`default_shell` 为 `cmd`）：**

```sh
# cmd.exe 下调 PowerShell 完成截图
remote-shell -conn win -c 'powershell -NoProfile -ExecutionPolicy Bypass -Command "Add-Type -AssemblyName System.Drawing,System.Windows.Forms; $b=New-Object Drawing.Bitmap([Windows.Forms.Screen]::PrimaryScreen.Bounds.Width,[Windows.Forms.Screen]::PrimaryScreen.Bounds.Height); [Drawing.Graphics]::FromImage($b).CopyFromScreen(0,0,0,0,$b.Size); $f=[IO.Path]::Combine([IO.Path]::GetTempPath(),[IO.Path]::GetRandomFileName()+\".png\"); $b.Save($f,[Drawing.Imaging.ImageFormat]::Png); cat $f; Remove-Item $f -ErrorAction SilentlyContinue"' > screen.png
```

## 键鼠输入

### Linux (X11)

```sh
# 鼠标移动并点击
remote-shell -conn linux -c 'DISPLAY=:0 xdotool mousemove 500 300 click 1'

# 键盘输入文本
remote-shell -conn linux -c 'DISPLAY=:0 xdotool type --delay 50 "hello world"'

# 快捷键
remote-shell -conn linux -c 'DISPLAY=:0 xdotool key Return'
remote-shell -conn linux -c 'DISPLAY=:0 xdotool key ctrl+a'

# ydotool（Wayland 兼容，需 root 或 input 组）
remote-shell -conn linux -c 'sudo ydotool mousemove --absolute 500 300 click 0xC0'
remote-shell -conn linux -c 'sudo ydotool key --delay 50 1 hello'
```

### macOS

```sh
# 鼠标点击（需 Accessibility 权限）
remote-shell -conn mac -c 'osascript -e "tell application \"System Events\" to click at {500,300}"'

# 键盘输入
remote-shell -conn mac -c 'osascript -e "tell application \"System Events\" to keystroke \"hello world\""'

# 快捷键
remote-shell -conn mac -c 'osascript -e "tell application \"System Events\" to keystroke \"a\" using command down"'
remote-shell -conn mac -c 'osascript -e "tell application \"System Events\" to key code 36"'  # Return

# 窗口操作
remote-shell -conn mac -c 'osascript -e "tell application \"Safari\" to activate"'
remote-shell -conn mac -c 'osascript -e "tell application \"System Events\" to tell process \"Safari\" to click button \"OK\" of window 1"'
```

### Windows

根据 `default_shell` 选择对应的命令：

**PowerShell（`default_shell` 为 `powershell`）：**

```sh
# 鼠标点击
remote-shell -conn win -c 'powershell -NoProfile -Command "Add-Type System.Windows.Forms; [System.Windows.Forms.Cursor]::Position=New-Object Drawing.Point(500,300); $c=[System.Windows.Forms.Cursor]::Position; Add-Type \"using System;using System.Runtime.InteropServices;public class M{[DllImport(\\\"user32.dll\\\")]public static extern void mouse_event(int f,int x,int y,int d,int t);}\"; [M]::mouse_event(0x0002,0,0,0,0); [M]::mouse_event(0x0004,0,0,0,0)"'

# 键盘输入
remote-shell -conn win -c 'powershell -NoProfile -Command "Add-Type System.Windows.Forms; [System.Windows.Forms.SendKeys]::SendWait(\"hello world\")"'
```

**cmd.exe（`default_shell` 为 `cmd`）：**

```sh
# 鼠标点击 — cmd 下调 PowerShell
remote-shell -conn win -c 'powershell -NoProfile -Command "Add-Type System.Windows.Forms; [System.Windows.Forms.Cursor]::Position=New-Object Drawing.Point(500,300); $c=[System.Windows.Forms.Cursor]::Position; Add-Type \"using System;using System.Runtime.InteropServices;public class M{[DllImport(\\\"user32.dll\\\")]public static extern void mouse_event(int f,int x,int y,int d,int t);}\"; [M]::mouse_event(0x0002,0,0,0,0); [M]::mouse_event(0x0004,0,0,0,0)"'

# 键盘输入 — cmd 下调 PowerShell
remote-shell -conn win -c 'powershell -NoProfile -Command "Add-Type System.Windows.Forms; [System.Windows.Forms.SendKeys]::SendWait(\"hello world\")"'
```

## 排障

| 现象 | 原因 | 解决 |
|------|------|------|
| 截图为全黑或全白 | 无活跃桌面或 Session 0 隔离 | 确认远端用户已登录 GUI 桌面；Windows 不要在锁屏状态下执行 |
| macOS 权限拒绝 | TCC 未授权 | 在 GUI 会话中手动运行一次 `screencapture`，授权 Screen Recording 和 Accessibility |
| Linux DISPLAY 空 | SSH 会话未设置 DISPLAY | 显式指定 `DISPLAY=:0`，或用 `ls /tmp/.X11-unix/` 查看可用显示 |
| No protocol specified | Xauthority 不匹配 | 设置 `XAUTHORITY=$HOME/.Xauthority` |
| Windows PowerShell 类型加载失败 | .NET 程序集未加载 | 确认 `Add-Type -AssemblyName System.Drawing` 成功 |
