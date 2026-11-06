# linux-lab — Linux 终端操作实操测验（TUI）

基于 [VLA-Tutorial-AGDR](https://github.com/GC-SHIRO/VLA-Tutorial-AGDR) **第二章「Linux终端操作」** 的可交互测验程序。

- **左侧**：题目说明与进度  
- **右侧**：内嵌真实 bash（PTY）+ **VT10x 终端仿真**（支持 `clear` / Ctrl+L、换行卷动、ANSI 高亮、闪烁光标）  
- **工作区**：`/tmp/linux-lab-<session>/`  
- **进度**：`~/.config/linux-lab/progress.json`  
- **交卷**：完整虚拟终端 IO history + 每题 PASS/FAIL JSON  
- **题目 init**：进入关卡时自动铺垫文件/后台进程  
- **简答**：`a` / `Ctrl+A` 提交（如 `du -hs` 大小）  
- **附加题**：`bonus` 标记，正式进度单独统计  

面向 **Ubuntu 24.04** 学生环境。

## 下载预编译二进制（推荐）

GitHub Actions 会在推送 `v*` 标签时交叉编译 **linux/amd64** 与 **linux/arm64**，并发布到 [Releases](https://github.com/kiramint/linux-lab-tui/releases)：

- `linux-lab-linux-amd64`
- `linux-lab-linux-arm64`
- `SHA256SUMS`

示例：

```bash
# AMD64（常见 x86_64 Ubuntu / 云主机）
curl -LO https://github.com/kiramint/linux-lab-tui/releases/download/v0.2.0/linux-lab-linux-amd64
chmod +x linux-lab-linux-amd64 && ./linux-lab-linux-amd64

# ARM64（树莓派 / 部分云主机）
curl -LO https://github.com/kiramint/linux-lab-tui/releases/download/v0.2.0/linux-lab-linux-arm64
chmod +x linux-lab-linux-arm64 && ./linux-lab-linux-arm64
```

也可用 Actions 的 **workflow_dispatch** 手动触发 Release 工作流（可选填标签名）。

## 编译

```bash
git clone https://github.com/kiramint/linux-lab-tui.git
cd linux-lab-tui
make build              # 生成本机 ./linux-lab
make build-linux-amd64  # 交叉编译 linux-lab-linux-amd64
make build-linux-arm64  # 交叉编译 linux-lab-linux-arm64
make build-all          # 同时打出 amd64 + arm64
# 或
go build -o linux-lab .
```

需要 Go 1.22+。

CI（`.github/workflows/ci.yml`）在 `main` 的 push/PR 上会跑 `go test ./...` 与 `go build`；正式产物由 `.github/workflows/release.yml` 构建。

## 运行

```bash
./linux-lab
./linux-lab list              # 列出题目
./linux-lab test-blocklist    # 安全拦截自检
```

### 快捷键

| 按键 | 作用 |
|------|------|
| `Shift+Tab` | 题目区 / Shell 区焦点切换 |
| `Tab` | Shell 焦点下交给 bash（命令补全） |
| `Ctrl+L` | 清屏（与 `clear` 相同，经 VT 仿真处理） |
| `PgUp` / `PgDn` | Shell 回滚缓冲（查看已滚出屏幕的行） |
| `Ctrl+G` | 判当前题 |
| `Enter` / `c` | 判当前题（题目焦点） |
| `a` / `Ctrl+A` | 输入简答（有 short_answer 的题） |
| `n` / `p` | 下一题 / 上一题（进入时自动跑 init） |
| `Ctrl+S` 或 `s` | 交卷 |
| `Ctrl+Q` | 退出（未交卷会确认） |
| `Ctrl+C` | Shell 焦点下发送给 bash |
| `Esc` / `Ctrl+[` | Shell 焦点下发给 PTY；简答编辑时取消 |

聚焦 Shell 时右边框高亮，并显示闪烁块光标。

**vim 提示**：退出 vim 用 `Esc` 后输入 `:q`（或 `:wq`）；若仍异常可用 `Ctrl+[`（终端里与 Esc 等价）。退出本测验程序请用 `Ctrl+Q`，不要用 Esc。

### 交卷产物

目录：`~/.config/linux-lab/submissions/`

- `<时间戳>-history.txt` — PTY 完整输入输出（含 `>>>` 命令行与 `[BLOCKED]` 记录）
- `<时间戳>-summary.json` — 各题通过情况

## 题目一览

内容覆盖：路径与文件、查看/搜索、管道重定向、权限与 chmod -R、chown/chgrp、ACL 附加、环境变量、进程（htop/PID）、tar、apt、**openssh-server**、systemctl enable/start/stop/status、scp、du/df/lsof、find 综合。

`Q11-openssh-server`、`Q12-systemctl-ssh` 等需要 **sudo**；请在真实 Ubuntu 24.04 虚拟机/实验机上完成：

```bash
sudo apt-get update
sudo apt-get install -y openssh-server
```

开发机/CI 若无 sudo 或未安装，相关题判 FAIL 属预期。

## 判题方式

1. **file** — 文件/目录是否存在、内容正则、权限 mode（如 `0700`）  
2. **command_regex** — 对学生键入命令（IO history 中 `>>>` 行）做正则匹配  
3. **output** — 在主机执行命令（如 `dpkg -l openssh-server`）并匹配 stdout  

## 安全拦截

发送到 shell 执行前（Enter 时）拦截，**不会执行**，并红字提示：

- `dd`
- `--no-preserve-root`
- `fdisk` / `cfdisk` / `parted`
- `rm -rf /` 及 `rm -rf /*`

## 技术栈

Go · [bubbletea](https://github.com/charmbracelet/bubbletea) · lipgloss · [creack/pty](https://github.com/creack/pty) · [hinshun/vt10x](https://github.com/hinshun/vt10x)（PTY 输出 → VT 网格 → 右栏渲染）

## 限制说明

- 嵌套在非 TTY / CI 中无法完整体验 TUI（可用 `list` / 单元测试 / `test-blocklist`）。
- Shell 输入按行做危险命令检测；复杂多行/特殊转义场景以练习题为准。
- openssh 相关题依赖学生机权限与网络（apt）。
- VT 仿真覆盖常用序列（清屏、光标、颜色、换行卷动）；完整 xterm 私有序列 / 复杂全屏 TUI（htop 等）未作为考试目标保证；vim 基本进出（Esc / Ctrl+[、:q）应可用。
- 回滚缓冲是滚出屏幕后的纯文本行，不含完整单元格属性。

## License

MIT
