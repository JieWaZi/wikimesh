# DevWiki Query 规则

> 仅当结构化搜索不足，需要第 3 档语义 query 时加载。本文件不替代 `devwiki.md` 的候选评分、证据路径和 view 读取协议。

## 命令边界

使用顶层命令：

```bash
wikimesh query <完整自然语言问题...> --project <project>
```

- 不再使用 `wikimesh qmd query` 作为 skill 查询入口；`qmd query` 只保留给 SDK/索引调试。
- 位置参数必须是一句完整问题，例如“用户管理的状态变化和操作流程是什么”，不要拆成多个关键词。
- `wikimesh query` 输出只是候选上下文，不是真相源；采用任何候选前必须回到 `wikimesh read <topic|workflow> <slug> --view card/core/explain --project <project>`。
- 如果缺少向量索引，在文档库本地 source 根目录执行或提醒维护者执行 `wikimesh qmd embed`；顶层 query 负责 `--project` 解析，qmd 维护命令仍是工作区命令。

## 参数填写

常用参数：

```bash
wikimesh query "<完整问题>" --project <project> --intent "<意图提示>" --limit 5 --candidate-limit 30
wikimesh query "<完整问题>" --project <project> --search-query "<辅助关键词>" --search-query "<正式术语>" --limit 8 --candidate-limit 60
wikimesh query "<完整问题>" --project <project> --explain --no-rerank
```

- `--intent` 用于告诉 query 扩展器本轮检索目标，优先写成领域动作和证据需求，不写成单个关键词。
- `--search-query` 是辅助 FTS 锚点，可重复传入 1-6 个；它只做关键词召回并参与 RRF 融合，不会为每个辅助关键词额外触发向量检索。
- `--collection` 只在明确知道 collection 名称且需要缩小范围时使用；普通 DevWiki 查询默认让 qmd 使用配置中的默认 collection。
- `--no-rerank` 适合快速探索或排查 reranker 问题；正式候选排序优先不加。
- `--explain` 只在调试 ranking、解释为什么命中某页或维护 query 污染时使用。

### 参数分工

Agent 必须先按以下分工填写参数，不要把同一批关键词同时塞进 question、intent 和 search-query：

| 参数 | 作用 | 应该写什么 | 不应该写什么 |
|---|---|---|---|
| `<完整问题>` | 作为 qmd hybrid query 的主语义，服务原始 FTS、原始向量、rerank 和 best chunk | 一句自然语言问题，包含主对象、关注维度和期望证据类型 | 纯关键词列表、多个无谓同义词、过长背景描述 |
| `--intent` | 提示检索目标，帮助 query expansion、rerank 和 chunk 选择 | “解释/定位/比较/排障/维护”这类动作 + 证据需求 | 单个关键词、页面标题、slug、文件路径 |
| `--search-query` | 显式 FTS 锚点，用于纠偏和补召回 | 正式术语、别名、接口名、配置项、错误码、短 slug 片段 | 泛词、完整自然语言句子、超过 6 个未筛选关键词 |

如果无法区分这三类内容，先回到 index/glossary 和已选目录做结构化定位，不要直接构造一个很长的 query。

### Question 写法

`<完整问题>` 应该回答“我要找什么上下文”，建议使用以下形态：

```text
<主题/现象> 的 <关注维度 A、B、C> 是什么？
<主题/接口/配置> 涉及哪些 <规则、状态、联动、实现入口>？
<现象> 的 <排查路径、可能原因、恢复动作> 在哪里有说明？
```

推荐：

```bash
wikimesh query "用户管理的配置、状态变化和实现入口是什么？" --project <project>
wikimesh query "权限管理的角色关系、状态变化和实现入口是什么？" --project <project>
wikimesh query "系统设置涉及哪些触发条件、状态流和操作流程？" --project <project>
```

不推荐：

```bash
wikimesh query 用户管理 权限管理 配置 状态 实现入口 --project <project>
wikimesh query "用户管理 权限管理 系统设置 角色关系 操作记录" --project <project>
```

### Search Query 锚点选择

`--search-query` 用来补足 FTS 锚点。数量宁少勿滥：

- 普通解释 / 单主题定位：1-3 个。
- 跨页面关系 / 排障链路：3-5 个。
- 超过 6 个说明范围没有收敛，必须先回到结构化搜索或 card 验证。

优先级：

1. glossary 正式术语或页面 title 中的业务名：`用户管理`、`权限管理`。
2. 稳定别名、配置项、接口名、错误码：`用户账号`、`角色关系`、`操作记录`。
3. 用户提供的明确锚点：日志关键字、API path、配置字段。
4. 谨慎使用泛词：`同步`、`配置`、`状态`、`策略` 只能作为补充，不能单独作为锚点。

如果一个锚点会明显引入其他业务族，例如只写 `同步` 会召回大量不同系统功能页面，应删除或替换为更具体的 `用户管理同步`、`权限策略同步`、`任务状态流转`。

## Intent 模板

按语义类型选择一个接近的 `--intent`，必要时补上项目术语：

| 语义 | `--intent` 建议 |
|---|---|
| `explain_topic` | `解释 Topic 的功能边界、配置规则、状态变化和联动关系` |
| `locate_code` | `定位 Workflow 的实现入口、调用链、配置处理和副作用` |
| `troubleshoot` | `查找排障线索、告警状态、异常条件和恢复流程` |
| `compare` | `比较候选 Topic 的边界差异，找出权威页面和不适用范围` |
| `relationship` | `查找多个 Topic 和 Workflow 之间的依赖、触发和数据流关系` |
| `wiki_maintenance` | `检查页面重复、边界冲突、过期内容和 query 污染` |

`--intent` 可以补充主题，但不要堆关键词。好的 intent 应该让 reranker 明白“什么证据更重要”：

```bash
--intent "解释 Topic 的功能边界、配置规则、状态变化和联动关系，并定位相关 Workflow 实现入口"
--intent "查找排障线索、告警状态、异常条件、恢复流程和相关实现入口"
--intent "比较候选 Topic 的边界差异，排除同名或泛词误召回页面"
```

## 查询形态

优先使用“完整问题 + 意图 + 少量辅助锚点”：

```bash
wikimesh query "用户管理涉及哪些配置、状态变化和实现入口？" --project <project> --intent "解释 Topic 的功能边界、配置规则、状态变化和联动关系" --search-query "用户管理" --search-query "权限管理" --limit 5 --candidate-limit 30
```

更完整的例子：

```bash
wikimesh query "用户管理的配置、状态变化和实现入口是什么？" \
  --project <project> \
  --intent "解释 Topic 的功能边界、配置规则、状态变化和联动关系，并定位相关 Workflow 实现入口" \
  --search-query "用户管理" \
  --search-query "权限管理" \
  --search-query "角色关系" \
  --limit 5 \
  --candidate-limit 30
```

不要把 query 当成多关键词 search 使用：

```bash
# 不推荐
wikimesh query 用户管理 权限管理 配置 状态 --project <project>
```

如果只是多关键词入口定位，继续使用更便宜的结构化搜索：

```bash
wikimesh search topic 用户管理 权限管理 配置 状态 --project <project>
wikimesh search workflow 用户管理 权限管理 配置 状态 --project <project>
```

## Limit 建议

- 普通解释、单主题定位：`--limit 5 --candidate-limit 30`
- 关系、联动、排障链路：`--limit 8 --candidate-limit 60`
- 维护审计、重复/冲突排查：`--limit 10 --candidate-limit 80 --explain`
- 快速探索或模型较慢：追加 `--no-rerank`

## 结果处理

1. 先把 query 输出当成候选池，只记录 `collection/path/title/score` 和必要 explain。
2. 对候选逐个回到 `wikimesh read ... --view card`，按 `devwiki.md` 的 Card Scoring、Competitor Check、Evidence Path 判定。
3. 只有 Evidence Path 为 high，或用户确认 medium 路径后，才读取 core/explain。
4. 不得直接引用 query 的 chunk 内容作为最终事实；最终依据必须落到 wiki 页面、raw 文件或已核对代码。

### 噪声处理

query 可能召回 `index.md`、`glossary.md`、`log.md`、`wiki/outputs/*` 或语义相近但业务族不同的页面。处理规则：

- `index.md` / `glossary.md` 只能作为入口线索，不能作为最终事实依据。
- `log.md` / `wiki/outputs/*` 只能作为维护线索，不能作为 active 事实依据。
- 与主问题业务族不同的 Workflow，即使 vector 分高，也必须进入 excluded，并说明“语义相似但主题不相关”。
- 优先采用 active Topic / Workflow；deprecated、proposal、report 页面不得作为主依据，除非用户明确问历史、提案或报告。

### Explain 判读

使用 `--explain` 时，按贡献来源判断结果质量：

- `Source=vec, QueryType=original`：完整问题的语义召回命中。
- `Source=fts, QueryType=original`：完整问题的关键词召回命中。
- `Source=fts, QueryType=search`：`--search-query` 辅助锚点命中。
- 只有泛词 `search-query` 命中的页面，置信等级最高只能是 medium。
- 同时有 original vector 和稳定锚点 FTS 的 active Topic/Workflow，优先进入 card 验证。

## Fallback

qmd 报错、超时、collection 未注册、向量缺失、cache 不可写或 reranker 不可用时，说明：

```text
本轮 query 不可用，已降级为 DevWiki 结构化入口搜索；结论只基于本轮可读证据。
```

随后回到 index/glossary、已选目录搜索和 card/core/explain 读取流程。
