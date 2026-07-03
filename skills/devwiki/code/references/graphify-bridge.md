# Graphify Bridge Gate

> DevWiki 原生搜索前的 Graphify 预检。Graphify 只给候选和已验证记忆；最终事实仍回到 `wikimesh read`、raw 或当前代码。

## Gate

每个用户问题进入 DevWiki 定位流程时，只执行一次 Gate；后续 `wikimesh search index/glossary/topic/workflow` 和 `wikimesh query` 复用同一个 `Graphify Context`。只有主题、project、code repo、查询目标发生变化，或用户要求重新分析时才重跑。

失败只降级，不阻断原流程。禁止直接读取、cat、解析或摘要 `graphify-out/*`、`GRAPH_REPORT.md`、`reflections/LESSONS.md`、`.graphify_learning.json`；只允许用 `test -f` / `find` 判断 graph 是否存在，graph 文件只能作为 `graphify` 命令参数。**记忆状态只通过 Gate 精简版 `graphify explain` 或 query 输出中的 `learning=` 获取，不得绕过 explain 直接读 LESSONS。**

## 可用性

```sh
OUT="${GRAPHIFY_OUT:-graphify-out}"
GRAPH="$OUT/graph.json"
command -v graphify
test -f "$GRAPH"
```

若 graph 在 Wiki source 或绑定代码仓下，切到对应根目录执行；也可以把 `GRAPH` 指向 `<repo>/graphify-out/graph.json`。`graphify` 不存在、graph 缺失、命令失败或无可映射候选时，输出 fallback 并继续原生流程。

## 输出体积控制

| 命令 | 精简方式 | 说明 |
|---|---|---|
| `graphify query` | `--budget 600`（Gate 默认） | 只召回 Wiki 候选时不必用 1500；宽范围可升到 800 |
| `graphify explain` | **无 `--budget`** | Gate 必须用管道截断，禁止消费完整 Connections |

Gate 专用 explain（只取 Lesson / Source，约 160 字符/节点）：

```sh
graphify explain "<节点 label>" --graph "$GRAPH" 2>/dev/null \
  | grep -E '^(Node:|  Source:|  Lesson:)'
```

需要节点元数据但不需要邻居时：

```sh
graphify explain "<节点 label>" --graph "$GRAPH" 2>/dev/null \
  | awk '/^Connections/{exit} {print}'
```

**禁止**在 Gate 中读取 explain 的 `Connections` 段；那是给 `locate_code` 深查用的，不是 Gate 必需。

## Gate 必做步骤（按序，不可跳步）

### Step 1 — 刷新 lessons + 召回候选

```sh
graphify reflect --if-stale --graph "$GRAPH"
graphify query "<用户问题>" --budget 600 --graph "$GRAPH"
```

### Step 2 — 检查 Wiki 候选的记忆状态（**必做，精简 explain**）

从 Step 1 的 query 输出中，提取可映射 Wiki 的节点（最多先保留 5 个）：

| query 中的 `src=` | 映射 |
|---|---|
| `topics/<slug>.md` | `topic/<slug>` |
| `workflows/<slug>.md` | `workflow/<slug>` |
| `troubleshooting/<slug>.md` | `troubleshooting/<slug>` |

优先保留 Topic 节点；`locate_code` 意图可额外保留 1–2 个 Workflow 节点。无法从 `src=` 映射 slug 的节点只能作线索，不能进入 fast path。

对保留的 Wiki 候选**串行**执行（最多 3 个），**只用精简 explain**：

```sh
graphify explain "<节点 label>" --graph "$GRAPH" 2>/dev/null \
  | grep -E '^(Node:|  Source:|  Lesson:)'
```

从 explain 输出解析 `Lesson:` 行：

| explain 信号 | `learned` |
|---|---|
| `Lesson: preferred` | `preferred` |
| `Lesson: tentative` | `tentative` |
| `Lesson: ... corrected` 或 correction 语义 | `corrected` |
| `dead_end` / known dead end | `dead_end` |
| `learning=...:stale` 或 `[code changed since` | `stale` |
| 正负混合 / contested | `contested` |
| 无 `Lesson:` | `none` |

若 query 输出已含 `learning=`，可与 explain 结果合并；**以 explain 为准**。

### Step 3 — 分支：fast path 或 fallback

**Fast path**（满足全部条件时才走）：

1. 至少 1 个 Wiki slug 的 `learned` 为 `preferred` 或 `corrected`，且非 `stale`
2. 该 slug 与用户 `intent_type` / `subject` 匹配（读 card 验证）
3. 读 core 后足以回答（窄范围单页够答；宽范围 `stable_broad` 时 **preferred slug 集合经 card/core 已形成最小覆盖**）

Fast path 动作：

```text
wikimesh read <type> <slug> --view card   # 逐个验证
wikimesh read <type> <slug> --view core   # primary + 全部 preferred supporting
→ 够答则 **禁止** 再执行 search index / search glossary / search topic|workflow
→ 仍不足时再降级补读 explain 或 fallback 到原生搜索
```

**Fast path 一致性（硬约束）**：

- 若执行了 `wikimesh search index/glossary/topic/workflow` 任一命令 → `path` 必须是 `fallback`，不得标 `fast_path`
- `path: fast_path` 时 `skipped` 必须明确列出已跳过的 index/glossary/topic search，**不得写「仍执行 index/glossary 确认」**
- 宽范围问题已有 ≥2 个 `preferred` Topic 且 card/core 覆盖用户 subject → 视为最小覆盖完成，**不需要** index/glossary 二次确认

**Tentative 试探**（仅 `stable_narrow` 且唯一候选时）：

- 只有 1 个 Wiki 候选为 `tentative`、无 competing preferred
- 读 card 匹配后读 core；够答可跳过 index/glossary
- 不匹配或 core 不足 → fallback

**Fallback**（以下任一即 fallback，Graphify 只做预热）：

- Graphify 不可用、无 Wiki 映射候选、explain 全为 `none`
- 仅有 `contested` / `stale` / `dead_end`（dead_end 节点排除，其余降权）
- card 不匹配、Evidence Path 为 medium/low、或 preferred 集合读 core 后仍不能回答

Fallback 动作：继续 `index → glossary → Scope Stability Check → …` 原生流程。Graphify 预热候选**不是**总候选上限，原生搜索可补充更多页面。

### Step 4 — 按需扩展命令

只在 fast path 不够或意图需要时使用：

```sh
graphify query "<用户问题>" --dfs --budget 1500 --graph "$GRAPH"
graphify query "<用户问题>" --context call --budget 1500 --graph "$GRAPH"
graphify path "<A>" "<B>" --graph "$GRAPH"
graphify explain "<node>" --graph "$GRAPH"   # 完整版，仅 locate_code 深查
```

`--dfs` / `path` 用于路径、连接、调用链问题。`--context call/import` 用于代码关系问题。

## 建图和更新

Gate 不自动建图。只有用户明确要求生成或更新图谱时才使用：

```sh
graphify .
graphify <path> --update
graphify <path> --mode deep
```

## 结果消费

- `devwiki-query`：Gate Step 2 精简 explain **不可跳过**。`preferred/corrected` 且 card+core 够答 → fast path，**禁止**再跑 index/glossary。否则 fallback。
- `devwiki-code`：Wiki 候选必须回到 workflow card/core；Code 候选必须回到真实代码核对，不能替代 `rg`、`sed`、测试。
- `devwiki-code-to-doc`：Graphify 只帮助找入口、邻接关系和页面归属；写入建议仍必须由当前代码、Wiki/raw 证据和 proposal 支撑。
- `contested/stale/dead_end` 不走 fast path；只作为排除、降权或提醒复核的信号。

Wiki 映射：`wiki/topics/<slug>.md -> topic`，`wiki/workflows/<slug>.md -> workflow`，`wiki/troubleshooting/<slug>.md -> troubleshooting`。

## Graphify Context（**仅 agent 内部，不对用户展示**）

Gate 结束后在 agent 内部记录，**不要**写入用户可见回答，除非用户明确要求调试 Graphify：

```markdown
Graphify Context (internal):
- availability: available | fallback(<reason>)
- path: fast_path | fallback
- skipped: none | index/glossary/topic search
- commands: <实际 graphify 命令>
- learned: preferred | corrected | tentative | contested | stale | dead_end | none
- preferred_slugs: <type/slug/title；无则 none>
- wiki_candidates: <最多 5 个>
- confidence: high | medium | low
```

用户可见回答只保留：语义识别、证据路径（Topic/Workflow 页面名）、正文结论、知识缺口。

## 学习反馈

只有结果被用户验证或高置信核对后才记录：

```sh
graphify save-result --question "<Q>" --answer "<A>" --type devwiki_query --nodes <Node...> --outcome useful
graphify save-result --question "<Q>" --answer "<A>" --type devwiki_query --nodes <Node...> --outcome dead_end
graphify save-result --question "<Q>" --answer "<A>" --type devwiki_query --nodes <Node...> --outcome corrected --correction "<正确方向>"
graphify reflect --if-stale --graph "$GRAPH"
```

不要在低置信、未核对或普通追问后保存。memory 文件会累积；lesson 影响力由 reflect 聚合和时间衰减管理，不自动删盘。
