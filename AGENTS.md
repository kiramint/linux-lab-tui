# AGENTS.md — 如何新增 / 修改题目

给改题的人（人或 agent）看的操作说明。题库是 YAML；判题逻辑在 `internal/grade`。学生环境默认 **Ubuntu 24.04**。

题源参考：[VLA-Tutorial-AGDR 第二章「Linux终端操作」](https://github.com/GC-SHIRO/VLA-Tutorial-AGDR/tree/main/2.%20Linux%E7%BB%88%E7%AB%AF%E6%93%8D%E4%BD%9C)（含 [05-用户与权限.ipynb](https://github.com/GC-SHIRO/VLA-Tutorial-AGDR/blob/main/2.%20Linux%E7%BB%88%E7%AB%AF%E6%93%8D%E4%BD%9C/05-%E7%94%A8%E6%88%B7%E4%B8%8E%E6%9D%83%E9%99%90.ipynb)）。

---

## 1. 题目放在哪

| 路径 | 作用 |
|------|------|
| `quests/chapter2.yaml` | 当前唯一题库（章节元数据 + `questions` 列表） |
| `main.go` 的 `//go:embed quests/chapter2.yaml` | 编译进二进制；**改 YAML 后必须重新 `make build` 才生效** |
| `internal/grade/quest.go` | `Bank` / `Question` / `Grader` 字段定义 |
| `internal/grade/grade.go` | 三种判题实现；一题内多个 grader **全部通过** 才 PASS |

工作区前缀来自 YAML 顶层 `workspace_prefix`（默认 `/tmp/linux-lab`），运行时会变成 `/tmp/linux-lab-<session>/`。题面里的相对路径都相对该工作区。Shell 启动时已 `cd` 到工作区，并设环境变量 `LINUX_LAB_WORKSPACE`。

若要换文件名或加第二套题库：改 `quests/` 下 YAML，并同步改 `main.go` 的 `go:embed`（以及 `ParseBank` 调用处）。只加文件却不 embed，发布二进制里**不会**带上新题。

---

## 2. Question 字段

```yaml
- id: Q99-example          # 稳定唯一 ID（进度/交卷按此键）
  title: "简短标题"         # 左侧列表显示
  section: "05 用户与权限"  # 分组标签
  needs_sudo: false         # true 时 UI 会标 [sudo]
  instructions: |           # 多行题面（支持 $LINUX_LAB_WORKSPACE 说明）
    在工作区完成……
  graders:                  # 非空；全部 OK 才算通过
    - type: file
      ...
```

顶层 bank：

```yaml
version: 1
title: "……"
workspace_prefix: "/tmp/linux-lab"
questions: [ ... ]
```

---

## 3. 三种 grader（`type`）

### 3.1 `file` — 文件是否存在 / 内容 / 权限

| 字段 | 说明 |
|------|------|
| `path` | 相对工作区路径（也可用绝对路径） |
| `exists` | `true`/`false`；**省略时默认要求存在** |
| `is_dir` | `true` 时要求是目录 |
| `content_regex` | Go regexp，匹配文件全文 |
| `mode_octal` | 如 `"0700"`，只比权限三位 |

```yaml
# 必须存在且内容含 ERROR
- type: file
  path: workspace/errors.txt
  exists: true
  content_regex: '(?m)ERROR'

# 必须已删除
- type: file
  path: practice/docs/readme.txt
  exists: false

# 目录 + 权限脚本
- type: file
  path: scripts/hello.sh
  exists: true
  content_regex: '(?i)echo.*hi'
  mode_octal: "0700"

- type: file
  path: practice/docs
  is_dir: true
```

### 3.2 `command_regex` — 学生是否输入过某类命令

匹配来源：会话 IO history 里以 `>>> ` 记录的按行发送内容，以及整段 history 兜底。

| 字段 | 说明 |
|------|------|
| `pattern` | Go regexp（常用 `(?i)` 忽略大小写） |
| `description` | 失败时展示的中文说明（建议写） |

```yaml
- type: command_regex
  pattern: '(?i)\bgrep\b.*ERROR'
  description: "曾执行含 ERROR 的 grep"

- type: command_regex
  pattern: '(?m)^\s*whoami\s*$'
  description: "执行过 whoami"
```

注意：只匹配**发到 PTY 的整行**；在 vim 里敲的内容、未回车的半行不会进 `>>>` 记录。

### 3.3 `output` — 判题时再跑一条命令看输出

由 grader 用 `bash -lc` 执行（不是学生当前 shell 状态里的临时变量，除非已写入文件）。

| 字段 | 说明 |
|------|------|
| `command` | 要执行的命令 |
| `cwd_workspace` | `true` 时在工作区目录执行 |
| `stdout_regex` | 匹配合并后的 stdout+stderr |
| `stdout_exact` | 与 trim 后全文精确相等（与 regex 二选一即可） |
| `description` | 人类可读说明 |

```yaml
- type: output
  command: "stat -c '%a' perms/note.txt"
  cwd_workspace: true
  stdout_regex: '(?m)^[0-7][0-7][4567]$'
  description: "others 具有读权限"

- type: output
  command: "dpkg -l openssh-server"
  cwd_workspace: false
  stdout_regex: '(?m)^ii\s+openssh-server'
  description: "dpkg 显示已安装"
```

`struct` 里还有 `mode_regex` 字段，**当前 `gradeFile` 未实现**，不要依赖。

---

## 4. 常见改题步骤

### 新增一题

1. 在 `quests/chapter2.yaml` 的 `questions:` 末尾（或目标位置）追加一个 `- id: ...` 块。
2. 选好 `graders`（可多条混用，见下）。
3. `id` 勿与已有重复；改 `id` 会导致旧进度对不上。
4. 本地验证（见 §5）→ commit → 需要发二进制时再打 tag（见 §7）。

### 改题面 / 难度

- 只改文案：动 `title` / `instructions` / `section`。
- 加难：收紧 `content_regex` / `pattern`，或增加 grader。
- 变易：放宽正则，或删掉过严的 `command_regex`（只留文件结果也可）。
- 需 sudo 的关卡设 `needs_sudo: true`，并在题面写清；CI/无 sudo 环境预期会 FAIL（见 `Q12-openssh-server`）。

### 调顺序

直接调整 YAML 里 `questions` 数组顺序；`linux-lab list` 与 TUI 左侧顺序跟数组一致。

### 一题多 grader

列表里**每一条都要 OK**（逻辑与）。典型组合：

```yaml
graders:
  - type: file
    path: archive/backup.tar.gz
    exists: true
  - type: command_regex
    pattern: '(?i)\btar\b.*\b(czf|-czf)'
    description: "使用 tar 创建过 gzip 归档"
  - type: output
    command: "tar -tzf archive/backup.tar.gz"
    cwd_workspace: true
    stdout_regex: '.+'
    description: "归档可列出内容"
```

---

## 5. 本地验证

```bash
make test          # go test ./...（含 ParseBank / 各 grader 单测）
make build         # 重新 embed YAML → ./linux-lab
make list          # 或 ./linux-lab list —— 确认 id/标题/顺序/[sudo]
./linux-lab test-blocklist   # 安全拦截自检（与题库无关）
./linux-lab        # 手动干跑：做一题 → Ctrl+G 看判题结果
```

干跑提示：

- 工作区在 `/tmp/linux-lab-<session>/`；进度在 `~/.config/linux-lab/progress.json`。
- 交过卷（Ctrl+S）后再启动会开新 session；未交卷且工作区还在会续用同一目录。
- 测 `command_regex` 时必须在右侧 shell **回车发送**整行，否则 history 无 `>>>`。
- 测 `output`/`dpkg`/`systemctl` 类题目需要真实 Ubuntu 包/服务；开发机没有则 FAIL 正常。

可选：在 `internal/grade` 加表驱动测试，用临时目录 + 伪造 history 字符串覆盖新正则。

---

## 6. 安全（与题库分离）

命令黑名单在 `internal/shell/blocklist.go`，**不是** YAML 字段。当前拦截包括：

- `dd`、`fdisk`、`cfdisk`、`parted`
- `--no-preserve-root`
- 危险的 `rm -rf /`（及类似写法）

改拦截规则：改 Go 里的 `blockPatterns`，并补 `internal/shell/blocklist_test.go`，再 `make test`。

出题时注意：

- **不要**在 `instructions` 里示范被拦命令。
- **不要**在 `output.command` 里跑破坏性命令（grader 会真执行）。
- `output.command` 保持只读探测（`stat` / `dpkg -l` / `tar -tzf` / `systemctl is-active` 等）。

---

## 7. 改完题库之后

1. `make test && make build && ./linux-lab list`
2. Commit 并 push 到 `main`（CI 会跑 test + build）。
3. **只有需要更新学生下载的二进制时**再发版：
   ```bash
   git tag v0.x.y && git push origin v0.x.y
   ```
   或 Actions 里手动跑 Release。产物：`linux-lab-linux-amd64` / `linux-lab-linux-arm64`。
4. 仅改文档、不改 embed 内容时，一般**不必**打新 tag。

---

## 速查：最小新题模板

```yaml
  - id: Q15-short-name
    title: "一句话标题"
    section: "综合"
    needs_sudo: false
    instructions: |
      1. ……
      2. 完成后 Ctrl+G 判题。
    graders:
      - type: file
        path: out/result.txt
        exists: true
        content_regex: 'ok'
      - type: command_regex
        pattern: '(?i)\becho\b'
        description: "使用过 echo"
```
