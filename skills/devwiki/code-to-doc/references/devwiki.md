# DevWiki 使用约束

> 供所有需要定位、读取或维护 DevWiki 项目知识的技能共享使用。

## 统一项目配置

1. 先从当前代码仓 `AGENTS.md` / `CLAUDE.md` 的 DevWiki link block 读取 `DevWiki project`。
2. 如果 link block 没有 project，执行 `wikimesh repo info`。该命令无参数时只输出已配置 project 名称 JSON 数组。
3. 只有一个 project 时直接使用；多个 project 时列出候选并让用户选择；没有配置时提示先执行 `wikimesh repo add <project> ...`。
4. 使用 `wikimesh repo info <project>` 确认统一项目配置。输出默认 JSON，`active_source` 是当前激活来源类型，决定使用 `sources.local` 还是 `sources.remote`；`sources` 保存该 project 已配置的 local/remote 来源；`code_repos` 提供已绑定代码仓路径。
5. 如果同一 project 已同时配置本地和远端，使用 `wikimesh repo use <project> <local|remote>` 切换当前来源，不要通过重复 `repo add` 覆盖另一边配置。
6. 后续 `wikimesh search/read` 默认使用 `--project <project>`。不要读取本地项目文件来推导文档库路径，也不要手工指定文档库根目录。

## 结构化入口优先，低置信升档

这里的“结构化入口优先”指 DevWiki 的 index/glossary 表格，不是代码仓全局搜索。

基础顺序：

```text
意图识别 → 必要时 glossary keywords 术语对齐 → wikimesh search index → wikimesh search glossary → wikimesh search topic/workflow → 必要时加载 query-rules.md 后使用 wikimesh query → raw/code 核对
```

任一阶段拿到足够强的 top-K 且置信足够即停；不要为了“保险”无边界扩大搜索。

### 意图识别

| 意图类型 | 典型问题 | 默认策略 |
|---|---|---|
| `locate_exact` | 文件在哪里、哪个函数定义、哪个接口注册、错误码在哪里 | 本地 Wiki / 必要时本地代码精确定位 |
| `explain_topic` | 某功能是什么、怎么工作、有哪些边界 | `wikimesh search index/glossary`；低置信或噪声大时升到 `wikimesh search topic` |
| `trace_implementation` | 怎么实现、调用链怎么走、状态写到哪里 | query 场景先读 Wiki/workflow；缺少代码锚点且明确要求当前代码核查时，建议显式使用 `$devwiki-code` |
| `troubleshoot` | 报错原因、不生效怎么查、日志从哪里来 | 先找 workflow/topic 候选；troubleshooting 可作为页面内容和导航线索，再按需核对 raw/code |
| `design_intent` | 为什么这么设计、整体架构是什么 | `wikimesh search index/glossary`；不足时 `wikimesh search`，再不足才加载 `references/query-rules.md` 使用 `wikimesh query` |
| `wiki_maintenance` | 页面是否重复、过期、冲突、query 是否会命中旧内容 | 本地 Wiki 审计 + `wikimesh search/status/update` 按需验证 |

不要把所有关键词都当成精确锚点。`ssh`、`vip`、`auth`、`token`、`query`、`sync` 这类短词只是中锚点；如果用户问“怎么实现 / 怎么设计 / 怎么排障”，不能因为本地命中这些短词就停止。

## 语义纠偏、候选评分与证据路径选择

用户确认门不是文档选择器，而是语义纠偏器。agent 必须基于 search 结果和 card 信息自主判断候选与用户意图的匹配程度，并选择最合适的单文档或多文档证据路径。用户只确认问题意图和业务范围，不负责选择文档。

在读取 core/explain、进入 code-tracing、回答事实结论、实现或维护前，必须完成以下流程。

### Step 1: Intent Profile

先生成用户语义画像，不要直接按搜索分数选文档。画像至少包含：

- `intent_type` / `subject` / `focus` / `scope` / `anchors` / `negative_scope`
- `intent_type`：`explain_topic` / `locate_code` / `troubleshoot` / `compare` / `relationship` / `public_answer` / `wiki_maintenance`
- `subject`：用户问题里的核心对象，例如功能名、配置项、接口、错误码、现象、页面标题或 slug
- `focus`：用户关注功能、配置、边界、状态、联动、实现入口、排障、差异还是对外口径
- `scope`：单主题、多主题关系、比较、跨流程、排障链路或维护审计
- `anchors`：用户提供的明确锚点
- `negative_scope`：用户没有要求的内容，例如未要求当前代码核查、未要求实现细节、未要求修改

### Step 2: Glossary Keywords 术语对齐

当 Intent Profile 的 `subject` / `anchors` 不稳定，或用户问题包含短词、简称、口语词、宽泛领域词、多个相邻主题时，先调用：

```bash
wikimesh glossary keywords --project <project>
```

该命令只逐行返回 `wiki/glossary.md` 的 `glossary` 列，用作项目术语先验和语义纠偏，不是真相源，也不能直接作为事实依据。agent 应从关键词列表中选出 0-5 个可能正式术语或别名，和用户原始问题一起作为后续 `search index/glossary/topic/workflow` 的查询词。

#### Glossary Alignment Gate

执行 `glossary keywords` 后，必须先形成术语对齐结论：

- `exact_term` / `candidate_terms` / `generic_terms` / `ambiguity`
- `exact_term`：用户原词是否命中 glossary 中的正式术语、slug、页面标题、接口、配置项或错误码
- `candidate_terms`：从 glossary 中选出的 0-5 个正式候选或常用别名
- `generic_terms`：用户词中只表示领域动作、不表示稳定主题的泛词，例如“探测、监控、同步、管理、配置、策略、查询”
- `ambiguity`：是否存在多个候选主题都能合理解释用户原词

搜索词构造规则：

- 如果有 `exact_term`，优先使用用户原词 + exact term 搜索。
- 如果没有 `exact_term`，但只有 1 个 `candidate_terms`，可使用用户原词 + candidate term 搜索，后续仍需 card 验证。
- 如果没有 `exact_term`，且有 2 个以上 `candidate_terms`，不得用 `generic_terms` 直接扩大搜索后组织事实答案；只能用 `candidate_terms` 做候选定位，读 card 后进入确认门。
- generic_terms 只能辅助召回，不能作为 primary subject；搜索结果如果主要由 generic_terms 命中，置信等级最高只能是 medium。

不要无脑每轮调用 `glossary keywords`。以下情况可以跳过：

- 用户已经给出明确 slug、页面标题、配置项、接口或错误码；
- 同一轮对话已有稳定 Evidence Path，追问没有改变 `subject` 和 `scope`；
- 当前任务是明确代码锚点或普通代码编辑，不需要 DevWiki 语义纠偏。

### Step 3: Card Scoring

候选摘要必须合并 search 结果和 card 信息：`type/slug/title/score/description` 来自搜索结果，`status/summary/confidence/命中意图/关键边界/summary` 来自 card。旧页面可能仍使用 `适合回答/不适合回答`，读取时要兼容。card 信息不足时不得把候选视为 high confidence。

对每个准备采用的候选执行 Card Scoring，并记录以下维度：

- `intent_match` / `subject_match` / `authority_match` / `card_fit` / `status_quality` / `ambiguity_penalty`
- `intent_match`：card 的“命中意图”或旧字段“适合回答”是否覆盖当前 `intent_type`
- `subject_match`：标题、description、summary 是否覆盖用户核心对象，而不是只命中同名词
- `authority_match`：Topic 是否用于功能边界，Workflow 是否用于实现入口，Troubleshooting 是否用于排障路径
- `card_fit`：summary、命中意图、关键边界是否支持或排斥当前问题；旧页面兼容读取适合回答、不适合回答
- `status_quality`：active / deprecated / report / proposal / troubleshooting 是否适合作为事实依据
- `ambiguity_penalty`：短词、宽泛词、同名配置项、同名接口路径、多候选相似、关系/比较问题被单点候选覆盖不全等风险

评分结果分为：

- `high`：语义、主题、权威类型和 card 均匹配，没有 Hard Veto，可作为 `primary` 或明确的 `supporting`
- `medium`：有关联，但角色、边界或主次关系不清，只能进入语义纠偏，不能直接当主依据回答
- `low`：只是关键词命中、card 不支持、类型不权威或状态有风险，不读取 core/explain，不组织事实答案

### Step 4: Competitor Check

候选 card 为 high 只说明该页面内部质量高，不代表用户语义已确认。Card Scoring 后必须检查外部竞争候选。

以下情况 Evidence Path 不得判为 high，最高只能为 medium，并必须进入确认门：

- 用户原词不是 glossary 正式术语、slug、页面标题、接口、配置项或错误码
- 存在 2 个以上 active 候选，且这些候选都能解释用户原词
- 用户词包含泛化动作词，例如“探测、监控、同步、管理、策略、配置”，但没有明确业务限定
- primary 候选只是“最像”，而不是和用户 `subject` 精确匹配
- 候选之间属于不同业务族，例如用户管理、权限管理、系统设置、审批流程同时出现

只有满足以下条件时，才能跳过确认继续读 core/explain：

- 用户 `subject` 精确命中正式术语、slug、标题、接口、配置项或错误码；
- 或者只有一个 active 候选，且没有合理竞争候选；
- 或者同一轮对话已经确认过同一个 `type + slug`，且用户追问没有改变 `subject` / `scope`。

### Step 5: Hard Veto

命中 Hard Veto 的候选不得作为 `primary`：

- card 的旧字段“不适合回答”明确包含当前 `intent_type` 或 `focus`
- card 的“关键边界”明确排除当前 `intent_type`、`subject` 或 `focus`
- 用户问 `explain_topic`，候选只是 Workflow、report 或 proposal，且没有独立 Topic 支撑
- 用户问 `troubleshoot`，候选没有 troubleshooting 或诊断信息，只是功能说明
- 用户问 `compare` / `relationship`，候选只能回答单点，且没有 supporting 集合
- 候选只是短词、宽泛词、同名配置项或同名接口路径命中，summary 没有覆盖用户核心对象
- deprecated / proposal / report 页面不能作为 active 事实主依据，除非用户明确问历史、提案或报告

### Step 6: Evidence Path

评分后必须构造 Evidence Path，而不是让用户选择文档：

- `primary` / `supporting` / `implementation` / `troubleshooting` / `excluded`
- 每个被采用页面必须说明 `role` 和 `why`
- `primary`：直接回答核心问题
- `supporting`：回答关联配置、边界、状态或联动点
- `implementation`：只提供文档记录的实现入口、模块职责或状态流线索
- `troubleshooting`：只提供排障现象、诊断路径、可能原因或恢复步骤
- `excluded`：命中但不采用，并说明排除原因，避免跑偏

单主题问题通常选择 1 个 `primary` 和 0-2 个 `supporting`。关系、联动、比较、排障和跨流程问题允许多个 Topic / Workflow / Troubleshooting 联合回答，但每个页面必须有明确角色，禁止把无角色的零散命中拼成新主题、新能力或推荐口径。

#### 用户可见候选展示

用户可见的候选名称必须优先使用页面 title、glossary 术语或业务中文名。不要写成 `用户管理 / user-management`、`权限设置 / permission-settings` 这类“名称 / slug”格式。

`slug` 只用于内部定位、证据路径记录和执行 `wikimesh read` 命令；除非用户明确询问页面路径、slug、维护定位或命令参数，否则不要把 slug 混入普通功能解释、候选列表或确认问题。

### Step 7: Confirmation Actions

根据证据路径置信等级分流：

- `high`：直接继续，简要说明证据路径。同一轮对话中，已确认或已说明的同一个 `type + slug` 可在后续追问复用，直到用户切换主题、指出方向不对、出现新的竞争候选或准备执行写入/实现等高风险动作。
- `medium`：只确认问题意图和业务范围，不让用户选择文档。说明 agent 的语义理解、主依据、相关依据和不确定点，询问“这个问题范围是否正确？”。
- `low`：停止事实回答，请用户补充业务锚点。明确反馈“本轮没有找到可靠匹配文档”，列出检索过的入口和可补充的功能名、配置项、接口、错误码、现象或页面标题。

medium / low 必须在读取 core/explain 前停止，不得先读取 core/explain 后再输出“可能是某能力”的事实答案。Medium confidence 确认模板：

```text
我理解你说的“<用户原词>”可能对应：
1. <候选 A>：<一句 card 摘要>
2. <候选 B>：<一句 card 摘要>
3. <候选 C>：<一句 card 摘要>

我倾向先按“<主候选>”理解，但这个词在当前 Project Brain 里不是稳定术语。你这里问的是“<主候选>”吗？
```

进入确认动作时，使用当前 Agent 环境的结构化用户确认工具：

- Codex 使用 askuserquestion / request_user_input
- Cursor 使用 AskQuestion / ask questions tool
- Claude 使用 AskUserQuestion

如果当前环境没有结构化提问工具，必须以普通聊天问题停止并等待用户回复，不得继续执行后续流程。用户未确认、否定问题范围、表示不确定、指出方向不对，或证据路径不足时，必须停止沿当前路径推进，改为请用户补充锚点、调整关键词或重新搜索。

没有可靠候选时，不要把分散命中综合成新主题、新能力或推荐口径；只允许说明未找到依据、列出检索过的入口和请求用户补充锚点。尤其是 explain_topic 场景，缺少独立 Topic 或高置信 Evidence Path 时，不能用多个页面里的同词命中拼出能力边界。


### 第 1 档：Index / Glossary

默认先检索 DevWiki 结构化入口：

```bash
wikimesh glossary keywords --project <project>
wikimesh search index <query...> --project <project>
wikimesh search glossary <query...> --project <project>
```

`glossary keywords` 逐行返回 glossary 关键词，只用于术语对齐。`search index` 默认输出 `|slug|type|description|` pipe table；`search glossary` 默认输出 `|slug|glossary|type|description|` pipe table；在 `--project` 下由 CLI 根据统一配置选择本地文档库或远端 HTTP API，不输出 `score`。agent 必须根据 `description`、card 和用户问题做语义打分。

结构化搜索置信判断：

| 置信等级 | 判断标准 | 后续动作 |
|---|---|---|
| high | index/glossary 命中 1-5 个入口；`type` 与意图一致；`description` 明确覆盖用户问题 | 唯一 high 且无竞争候选时可说明依据后读取命中页；多个 high 或边界不清时进入用户确认门 |
| medium | 命中 6-20 条；有 2-4 个候选入口；需要读 card 后判断主页面 | 必要时只读 card 帮助排序，然后进入用户确认门；仍无法排序则升档 |
| low | 0 命中；超过 20 条散点命中；短词命中过泛；active/deprecated/report 混杂；页面冲突；无法判断权威页 | 必要时先用 `glossary keywords` 做术语对齐，再升档到 `wikimesh search <topic|workflow> <query...> --project <project>`；升档后找到候选再进入 Evidence Path；仍无可靠候选时再向用户反馈并请求补充锚点 |


### 第 2 档：Topic / Workflow Search

当 index/glossary 低置信、噪声过大、无法排序，或问题本身偏语义/主题类时，使用 `wikimesh search topic/workflow`：

```bash
wikimesh search topic <query...> --project <project>
wikimesh search workflow <query...> --project <project>
wikimesh search workflow 用户管理 权限设置 审批流程 --project <project>
```

- 多个关键词应作为多个参数传入，不要合并成一个带空格的字符串。
- 多 query 结果使用 RRF（Reciprocal Rank Fusion）按各关键词下的排名融合排序，`score` 为融合后的相对分。
- `wikimesh search topic/workflow` 底层调用 `qmd search`，不依赖向量，CPU 友好，适合作为结构化入口搜索的升档。
- `wikimesh search topic/workflow` 默认输出 `|slug|title|score|` pipe table。
- `wikimesh search` 命中只是候选排序，最终结论必须回到真实 `wiki/`、`raw/` 或已核对代码文件。

### 第 3 档：Semantic Query

只有 `wikimesh search` 和结构化入口搜索仍不足，且问题本身是概念、设计、意图、跨页面关系、排障链路或维护审计类时，才加载 `references/query-rules.md` 并使用 `wikimesh query <question...> --project <project>`。

- `wikimesh query` 的位置参数是一句完整自然语言问题，不是多个关键词。
- 多关键词召回继续优先使用 `wikimesh search topic/workflow <query...>`。
- query 参数、`--search-query` 辅助锚点、`--intent` 写法、limit 建议和 fallback 规则全部放在 `references/query-rules.md`，需要进入第 3 档时再加载。

### qmd 失败 fallback

qmd 报错、超时、collection 未注册或 cache 不可写时，说明：

```text
本轮 query 不可用，已降级为 DevWiki 结构化入口搜索；结论只基于本轮可读证据。
```

## 视图分层读取

读取 Topic / Workflow 时先用 card 判断，再按 Evidence Path 置信等级读取 core/explain：

```bash
wikimesh read <topic|workflow> <slug> --view card --project <project>
wikimesh read <topic|workflow> <slug> --view core --project <project>
wikimesh read <topic|workflow> <slug> --view explain --project <project>
```

- `slug` 必须使用 search 结果中的 `slug` 字段，不要用文件名。
- 候选逐个 card 验证；只有 Evidence Path 为 high，或用户确认 medium 路径后，才读取 core/explain；不要并行读多个候选的 card。
- 只读 query 类 skill 必须用 `wikimesh read`，禁止直接读取 topic/workflow 文件。
- 写入类 skill 已确认需要修改本地 Wiki 文件时，可以读取目标本地 Markdown 文件；因为目标就是修改该文件。remote source 不执行本地写入。

## Query Principles

1. 不凭空回答项目事实。
2. 先查 Wiki 和 raw 来源；query 不自动核对真实代码。
3. 每个关键结论都有来源。
4. explain_topic 回答禁止直接展开代码、函数、文件路径、调用链、测试入口或修改方式。
5. locate_code 只进入 Workflow 文档层面；locate_code 默认回答 Workflow 文档中的实现入口、模块职责、状态流/数据流、副作用和文档中已记录的代码线索。
6. 如果文档不足以支撑当前代码事实，不要静默查代码；说明知识缺口。缺少代码锚点且需要 DevWiki 定位入口时，建议显式使用 `$devwiki-code` 做代码核查；已有明确锚点时，按普通代码查看处理。
8. 读取 view 时遵守知识经济学放置规则：先用 card 判断命中，再用 core 回答主问题，只有 core 不够时读取 explain。
9. 搜索和读取串行降级，不并行：`wikimesh search index` → `wikimesh search glossary` → `wikimesh search topic/workflow`，候选逐个 card 验证，只有 Evidence Path 为 high 或用户确认 medium 路径后才往下走。
10. 没有可靠候选、没有独立 Topic/Workflow 或用户未确认候选时，不组织推断性答案；只输出“当前 Project Brain 没有足够信息支持该结论。”、检索过的入口和可补充的锚点。

## 目录选择规则

| 用户意图 | 用户实际在问 | 优先目录 | 辅助目录 |
|---|---|---|---|
| explain_topic | 能力边界、功能规则、配置、状态、联动、流程规则 | `wiki/topics/` | `wiki/glossary.md`, `raw/` |
| locate_code | 文档记录的代码入口、调用链、接口、内部逻辑、实现机制、影响范围 | `wiki/workflows/` | `wiki/topics/`, `workflow explain` |
| troubleshoot | 报错、不生效、怎么排查、怎么修复 | `wiki/troubleshooting/` | `wiki/workflows/`, `wiki/topics/` |
| public_answer | 对外说明、客户口径、官网/文档口径 | public 可见 Topic | 不读代码 |

典型顺序：

- 能力/功能问题：`topics`
- 实现问题：`workflows → topics → workflow explain`
- 排障问题：`troubleshooting → workflows → topics`

## 去重与权威来源规则

- 能力边界、功能规则、参数、联动和设计流转以 topic 页面为准。
- 调用链、关键逻辑、代码定位、修改影响和测试入口以 workflow 页面为准。
- 日志、错误码、排障步骤以 troubleshooting 页面为准。
- 其他页面如果出现重复内容，只能作为摘要和导航，不作为权威来源。
