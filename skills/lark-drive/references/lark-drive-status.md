
# drive +status

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

按 SHA-256 内容哈希比较一份本地文件清单与飞书云空间文件夹，输出四类差异：

| 字段 | 含义 |
|------|------|
| `new_local` | 仅本地清单中存在 |
| `new_remote` | 仅云端存在 |
| `modified` | 双端都存在但 hash 不一致 |
| `unchanged` | 双端都存在且 hash 一致 |

只读命令：流式 hash，不下载落盘；但双端都有的文件会从云端拉一份字节流过来在内存里算 hash，大目录 / 大文件会有可观的网络流量。

## 责任分离 —— 本地遍历由你负责

`+status` 不会自己 walk 本地目录。**你把要参与对比的本地文件清单（manifest）通过 `--files-from` 传进来**，命令负责逐个 hash + 与云端 list 比对。

这样设计的好处：

- 你可以用 `find -name '*.md'` 只比对 markdown，用 `find ... -newer ref` 只比对最近改过的，用 `git ls-files` 跟着仓库 untracked 的过滤口径走，等等。
- shortcut 不需要重复发明一套 `--ext` / `--include` / `--exclude` / `.gitignore` 解析；shell 的标准 flag 就够。
- 命令本身保持小、纯：hash + diff。

## 命令

```bash
# 把整个本地目录当作 manifest（最常见用法）
(cd ./local && find . -type f) | \
  lark-cli drive +status \
    --local-dir ./local \
    --folder-token fldcnxxxxxxxxx \
    --files-from -

# 只比对 markdown
(cd ./local && find . -type f -name '*.md') | \
  lark-cli drive +status \
    --local-dir ./local \
    --folder-token fldcnxxxxxxxxx \
    --files-from -

# 排除目录
(cd ./local && find . -type f -not -path '*/node_modules/*' -not -path '*/.git/*') | \
  lark-cli drive +status \
    --local-dir ./local \
    --folder-token fldcnxxxxxxxxx \
    --files-from -

# 用 git ls-files（跟仓库 tracked 范围对齐）
(cd ./local && git ls-files) | \
  lark-cli drive +status \
    --local-dir ./local \
    --folder-token fldcnxxxxxxxxx \
    --files-from -

# 从已有清单文件读
lark-cli drive +status \
  --local-dir ./local \
  --folder-token fldcnxxxxxxxxx \
  --files-from @manifest.txt
```

## 参数

| 标志 | 必填 | 类型 | 说明 |
|------|------|------|------|
| `--local-dir` | 是 | path | 本地根目录（**相对于 cwd**，不接受绝对路径） |
| `--folder-token` | 是 | string | Drive 文件夹 token |
| `--files-from` | 是 | path / `-` | 本地文件清单：`-` 从 stdin 读，`@path` 从文件读，每行一个相对路径 |

`--files-from` 接收的每一行允许以下几种形式（自动归一化为相对 `--local-dir` 的 forward-slash 路径）：

- `a/b.txt`（已经是 root 内的相对路径）
- `local/a/b.txt`（带 root 前缀，自动剥离）
- `./local/a/b.txt`（`find ./local -type f` 的标准输出，自动剥离）

逃出 `--local-dir` 的路径（含 `..`）会被拒绝。

## 输出 schema

```json
{
  "new_local":  [{"rel_path": "..."}],
  "new_remote": [{"rel_path": "...", "file_token": "..."}],
  "modified":   [{"rel_path": "...", "file_token": "..."}],
  "unchanged":  [{"rel_path": "...", "file_token": "..."}]
}
```

`rel_path` 始终用 `/` 分隔。仅本地存在时没有 `file_token` 字段。

## 比较范围

- **只比对 Drive `type=file` 的二进制文件**。在线文档（`docx` / `sheet` / `bitable` / `mindnote` / `slides`）和快捷方式（`shortcut`）都被跳过 —— 它们没有等价的本地二进制可对齐，否则会在 `new_remote` 里产生大量误报。
- 子文件夹会递归遍历；rel_path 形如 `sub1/sub2/file.txt`。
- 本地侧仅 hash 你 manifest 里指定的文件 —— 不会自动发现 manifest 之外的内容。

### manifest 不影响云端 list 范围

`--files-from` 只决定**本地侧** hash 哪些文件，不影响云端 list。这意味着如果你用 `find -name '*.md'` 把 manifest 限制成只 `.md`，云端文件夹里的所有非 `.md` 文件仍然会全部进入 `new_remote` 桶（"本地清单里没列、云端有"）。

如果你想做对称过滤（"只关心 `.md` 在两端的状态，云端别的都不看"），用 `jq` 在结果上再过滤一道：

```bash
... | lark-cli drive +status ... --files-from - | jq '
  .new_remote |= map(select(.rel_path | endswith(".md")))
'
```

或者反过来想：`new_remote` 默认全报反而是个有价值的信号 —— 它告诉你"云端还有这些你 manifest 漏掉的内容"。

## 典型用法

把 +status 当作"先看差异、再决定怎么同步"的只读探针：

- 想知道云端有什么本地没有的内容 → 看 `new_remote`，按需 `drive +download --file-token <token>`。
- 想把本地新增的内容推到云端 → 看 `new_local`，再 `drive +upload --file <path> --folder-token <parent>`（注意 +upload 不接受 0 字节文件）。
- 想知道哪些文件在云端被同事改过 → 看 `modified`，逐个 `drive +download` 对照内容。

## 性能注意

- `unchanged` + `modified` 的总字节数 = 本次需从云端下载的流量。100GB 的双端共享内容意味着 100GB 网络往返。
- 仅一侧存在的文件不会被下载。
- Hash 计算在内存里流式做（io.Copy → sha256.New），不会把云端文件落到磁盘。

## 所需 scope

| 操作 | scope |
|------|-------|
| 列出文件夹 / 子目录 | `drive:drive.metadata:readonly` |
| 下载并 hash 文件 | `drive:file:download` |

如果当前 token 缺这些 scope，命令会直接报 `missing_scope` 并提示重新登录。`drive:drive` 在部分企业被策略禁用，所以 +status 故意只声明上面这两个细粒度 scope。

## 参考

- [lark-drive](../SKILL.md) —— 云空间全部命令
- [lark-shared](../../lark-shared/SKILL.md) —— 认证和全局参数
- [lark-drive-upload](lark-drive-upload.md) / [lark-drive-download](lark-drive-download.md) —— 把 +status 输出接到推/拉动作上
