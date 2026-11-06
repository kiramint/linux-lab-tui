# AGENTS.md — 如何新增 / 修改题目

给改题的人（人或 agent）看的操作说明。题库是 YAML；判题逻辑在 `internal/grade`。学生环境默认 **Ubuntu 24.04**。

题源参考：[VLA-Tutorial-AGDR 第二章「Linux终端操作」](https://github.com/GC-SHIRO/VLA-Tutorial-AGDR/tree/main/2.%20Linux%E7%BB%88%E7%AB%AF%E6%93%8D%E4%BD%9C)（含 [05-用户与权限.ipynb](https://github.com/GC-SHIRO/VLA-Tutorial-AGDR/blob/main/2.%20Linux%E7%BB%88%E7%AB%AF%E6%93%8D%E4%BD%9C/05-%E7%94%A8%E6%88%B7%E4%B8%8E%E6%9D%83%E9%99%90.ipynb)）。

当前版本：**0.2.0**（支持每题 `init`、简答 `short_answer`、`bonus` 附加题）。

---

## 1. 题目放在哪

| 路径 | 作用 |
|------|------|
| `quests/chapter2.yaml` | 当前唯一题库（章节元数据 + `questions` 列表） |
| `main.go` 的 `//go:embed quests/chapter2.yaml` | 编译进二进制；**改 YAML 后必须重新 `make build` 才生效** |
| `internal/grade/quest.go` | `Bank` / `Question` / `Grader` 字段定义 |
| `internal/grade/grade.go` | 判题实现（`file` / `command_regex` / `output` / `short_answer`）+ `RunInit` |
| `internal/progress/progress.go` | 进度：`Results`、`Inited`、`Answers` |
| `internal/tui/model.go` | 切题跑 init、简答输入、附加题展示 |

工作区前缀来自 YAML 顶层 `workspace_prefix`（默认 `/tmp/linux-lab`），运行时会变成 `/tmp/linux-lab-<session>/`。题面里的相对路径都相对该工作区。Shell 启动时已 `cd` 到工作区，并设环境变量 `LINUX_LAB_WORKSPACE`。

若要换文件名或加第二套题库：改 `quests/` 下 YAML，并同步改 `main.go` 的 `go:embed`（以及 `ParseBank` 调用处）。只加文件却不 embed，发布二进制里**不会**带上新题。

---

## 2. Question 字段

```yaml
- id: Q99-example          # 稳定唯一 ID（进度/交卷按此键）
  title: "简短标题"         # 左侧列表显示
  section: "05 用户与权限"  # 分组标签
  needs_sudo: false         # true 时 UI 会标 [sudo]
  bonus: false              # true 时 list/UI 标 [附加]；正式通过数不含附加
  answer_prompt: ""         # 非空则提示学生按 a/Ctrl+A 输入简答
  init: |                   # 可选：进入本题时自动执行的 bash（cwd=工作区）
    mkdir -p demo
    echo x > demo/a.txt
  init_once: true           # 默认 true；同一 session 同一题只跑一次
  instructions: |           # 多行题面
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

### 2.1 `init` — 题目初始化脚本

- 由程序用 `bash -lc` 在**工作区**执行（`Dir=workspace`，并设置 `LINUX_LAB_WORKSPACE`）。
- **不经过学生 PTY**，因此**不受学生命令黑名单限制**（init 里可用 `dd` 造大文件）。
- 默认 `init_once: true`：每个题 ID 每个 session 只成功初始化一次（进度里 `Inited` 记录）。
- 触发时机：首次显示当前题（启动）、以及 `n`/`p` 切到该题时。
- 失败时状态栏：`初始化失败: …`；成功：`已初始化环境`。
- 后台辅助进程（如 sleep / lsof holder）：把 PID 写到 `.lab/qXX.pid`，题面说明进程名；init 应幂等（pid 仍存活则跳过）。
- **后台进程必须用 `setsid ... &`**（并重定向 stdin/stdout/stderr），否则 `RunInit` 的 `bash -lc` 会一直等到该进程结束才返回。
- **安全**：只在工作区内操作，禁止抹盘/`rm -rf /` 等。

### 2.2 `bonus` — 附加题

- `bonus: true` → `./linux-lab list` 与左侧题面显示 `[附加]`。
- 进度条「正式 X/Y」不含附加；附加仍可判题、仍写入 Results。

### 2.3 简答键位

| 键 | 作用 |
|----|------|
| `a`（题目焦点） | 进入简答编辑 |
| `Ctrl+A`（全局） | 同上 |
| `Enter` | 保存到 `Answers[题ID]` |
| `Esc` | 取消编辑 |

左侧显示当前已保存答案。`Ctrl+G` 判题时把该答案交给 `short_answer` grader。

---

## 3. 四种 grader（`type`）

### 3.1 `file` — 文件是否存在 / 内容 / 权限

| 字段 | 说明 |
|------|------|
| `path` | 相对工作区路径（也可用绝对路径） |
| `exists` | `true`/`false`；**省略时默认要求存在** |
| `is_dir` | `true` 时要求是目录 |
| `content_regex` | Go regexp，匹配文件全文 |
| `mode_octal` | 如 `"0700"`，只比权限三位 |

```yaml
- type: file
  path: workspace/errors.txt
  exists: true
  content_regex: '(?m)ERROR'

- type: file
  path: practice/docs/readme.txt
  exists: false
```

`struct` 里还有 `mode_regex` 字段，**当前 `gradeFile` 未实现**，不要依赖。

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
```

注意：只匹配**发到 PTY 的整行**；在 vim 里敲的内容、未回车的半行不会进 `>>>` 记录。

### 3.3 `output` — 判题时再跑一条命令看输出

由 grader 用 `bash -lc` 执行（不是学生当前 shell 状态里的临时变量，除非已写入文件）。

| 字段 | 说明 |
|------|------|
| `command` | 要执行的命令 |
| `cwd_workspace` | `true` 时在工作区目录执行 |
| `stdout_regex` | 匹配合并后的 stdout+stderr |
| `stdout_exact` | 与 trim 后全文精确相等 |
| `description` | 人类可读说明 |

```yaml
- type: output
  command: "stat -c '%a' perms/note.txt"
  cwd_workspace: true
  stdout_regex: '(?m)^[0-7][0-7][4567]$'
  description: "others 具有读权限"
```

### 3.4 `short_answer` — 学生键入的简答

配合 `answer_prompt` 与 UI 简答框。空答案 → FAIL「未作答」。

| 字段 | 说明 |
|------|------|
| `expect_command` | **推荐**：在工作区跑命令，stdout trim 后与学生答案比 |
| `expect_exact` | 与答案精确相等（先 normalize） |
| `expect_regex` | 答案匹配该正则 |
| `normalize` | 默认/`trim`：去首尾空白与换行 |
| `description` | 说明 |

优先级：有 `expect_command` 时以命令输出为准；否则 `expect_exact`；否则 `expect_regex`。

```yaml
answer_prompt: "请输入 du -hs bigdir 的第一列大小（如 2.0M）"
graders:
  - type: command_regex
    pattern: '(?i)du\s+-hs\b'
    description: "执行过 du -hs"
  - type: short_answer
    expect_command: "du -hs bigdir | awk '{print $1}'"
    normalize: trim
    description: "目录大小"
```

动态题（du/df）务必用 `expect_command`，避免写死机器相关数值。

---

## 4. 常见改题步骤

### 新增一题

1. 在 `quests/chapter2.yaml` 的 `questions:` 目标位置追加 `- id: ...`。
2. 需要铺垫文件时写 `init:`；需要学生填数字/字符串时写 `answer_prompt` + `short_answer`。
3. `id` 勿与已有重复；改 `id` 会导致旧进度对不上。
4. 本地验证（见 §5）→ commit → 需要发二进制时再打 tag（见 §7）。

### 改题面 / 难度

- 只改文案：动 `title` / `instructions` / `section`。
- 加难：收紧正则，或增加 grader。
- 变易：放宽正则，或用 `init` 减少前置手工。
- 需 sudo 的关卡设 `needs_sudo: true`。

### 调顺序

直接调整 YAML 里 `questions` 数组顺序；`linux-lab list` 与 TUI 左侧顺序跟数组一致。

### 一题多 grader

列表里**每一条都要 OK**（逻辑与）。典型组合：`init` 铺垫 + `command_regex` + `file`/`output`/`short_answer`。

---

## 5. 本地验证

```bash
make test          # go test ./...（含 ParseBank / RunInit / short_answer）
make build         # 重新 embed YAML → ./linux-lab
make list          # 或 ./linux-lab list —— 确认 id/标题/顺序/[sudo]/[附加]
./linux-lab test-blocklist
./linux-lab        # 干跑：切题看 init → a 简答 → Ctrl+G
```

干跑提示：

- 工作区在 `/tmp/linux-lab-<session>/`；进度在 `~/.config/linux-lab/progress.json`（含 `inited`、`answers`）。
- 交过卷（Ctrl+S）后再启动会开新 session；未交卷且工作区还在会续用同一目录。
- 测 `command_regex` 时必须在右侧 shell **回车发送**整行。
- 测 `output`/`dpkg`/`systemctl` 类题目需要真实 Ubuntu 包/服务。
- 后台进程题（Q08/Q16）依赖 init 拉起的 helper；重复进入同 session 应幂等。

---

## 6. 安全（与题库分离）

命令黑名单在 `internal/shell/blocklist.go`，**不是** YAML 字段。当前拦截包括：

- `dd`、`fdisk`、`cfdisk`、`parted`
- `--no-preserve-root`
- 危险的 `rm -rf /`（及类似写法）

**例外**：题目 `init` 在 grader 进程内执行，**不走**黑名单（可 `dd` 造 `bigdir`）。

出题时注意：

- **不要**在 `instructions` 里示范被拦命令（除非说明「init 已造好」）。
- **不要**在 `output.command` / `expect_command` 里跑破坏性命令。
- `init` 只写工作区相对路径。

---

## 7. 改完题库之后

1. `make test && make build && ./linux-lab list`
2. Commit 并 push 到 `main`（CI 会跑 test + build）。
3. **需要更新学生下载的二进制时**再发版：
   ```bash
   git tag v0.x.y && git push origin v0.x.y
   ```
   或 Actions 里手动跑 Release。产物：`linux-lab-linux-amd64` / `linux-lab-linux-arm64`。
4. 仅改文档、不改 embed 内容时，一般**不必**打新 tag。

---

## 速查：带 init + 简答的模板

```yaml
  - id: Q15-short-name
    title: "一句话标题"
    section: "综合"
    needs_sudo: false
    bonus: false
    answer_prompt: "请输入 ……"
    init: |
      mkdir -p out
      echo ready > out/seed.txt
    instructions: |
      1. ……
      2. 简答用 a / Ctrl+A；完成后 Ctrl+G。
    graders:
      - type: command_regex
        pattern: '(?i)\becho\b'
        description: "使用过 echo"
      - type: file
        path: out/result.txt
        exists: true
      - type: short_answer
        expect_command: "cat out/seed.txt"
        normalize: trim
        description: "seed 内容"
```
