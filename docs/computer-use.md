# 远程桌面操作

`remote-computer-use` 通过 `remote-shell` 截图并发送键鼠输入，适用于需要视觉反馈的浏览器和桌面应用任务。先确认目标连接与交互会话，再按“观察 → 操作 → 验证”的循环推进。

完整说明随技能一起安装，仓库中的维护入口如下：

- [技能操作流程](../plugins/skills-canonical/remote-computer-use/SKILL.md)：新截图、焦点、坐标映射、重试和结果验证。
- [Linux](../plugins/skills-canonical/remote-computer-use/references/linux.md)：X11 的 DISPLAY/XAUTHORITY、截图与输入，以及 Wayland 的能力边界。
- [macOS](../plugins/skills-canonical/remote-computer-use/references/macos.md)：截图、AppleScript、Unicode 输入与桌面权限。
- [Windows](../plugins/skills-canonical/remote-computer-use/references/windows.md)：交互会话隔离、PowerShell 编码、PNG 传输和虚拟桌面坐标。

首次使用请先[安装两个技能](../INSTALL.md)，配置 SSH 连接。SSH 命令能够执行，并不保证该进程能访问桌面；若截图失败或输入没有效果，先确认会话、权限和实际显示设备。
