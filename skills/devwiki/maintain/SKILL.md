---
name: "devwiki-maintain"
description: "当需要维护 DevWiki 的证据一致性、知识健康、过期内容、引用缺失、冲突遗漏或 query 命中质量时使用。"
argument-hint: "<待维护范围，例如 wiki 全量、某个 topic/workflow、某个 raw 文件、某次 query 失败案例>"
---

# /devwiki-maintain

## 核心定位

Maintain 的目标不是“整理格式”，而是保证 query 后续使用的是当前正确知识。

普通代码编辑、当前 diff 调整或用户已给出明确 patch 时，不接管；这类请求按普通编辑任务处理。

```text
raw/source/code = 证据层，只读，不直接改
wiki 当前页       = 当前理解层，可以维护和重写
outputs/report   = 维护过程报告，不是事实入口
index/glossary   = query 入口控制层
```

开始前按需读取：

- `references/devwiki.md`：定位 project、搜索关联文档、读取 view、判断目录和权威来源。
- `references/evidence-grounding.md`：判断事实、推断、来源和冲突。
- `references/code-tracing.md`：涉及代码核对、代码归因或最新实现时读取。
- `references/mutation-safety.md`：涉及删除、重命名、拆分、合并、破坏性操作或高风险冲突时读取。
- `references/common-file-format.md`：涉及 `wiki/index.md`、`wiki/glossary.md` 或 `wiki/log.md` 更新时读取。
- 需要语义升档时，按 `references/devwiki.md` 直接使用 `wikimesh qmd query`；需要向量检索前先执行或确认 `wikimesh qmd embed`。

## 维护目标

该 Skill 要发现并修正：

- Wiki 与 raw/source/code 不一致；
- Wiki 使用旧机制、旧结论、旧配置；
- raw 中关键设计点被 Wiki 漏掉；
- Wiki 过度摘要，导致关键规则、边界、状态、配置、异常场景丢失；
- 多个 Wiki 页面之间存在冲突；
- 多个 raw/source/code 对同一结论描述不一致；
- Wiki 结论缺少来源；
- 设计稿与代码实现存在差异但未归位；
- 过期页面仍可能被 qmd/query 检索并用于回答；
- index/glossary 或页面入口链接未同步，导致 Agent 走错入口。

## 写入模式

| 模式 | 触发 | 行为 |
|---|---|---|
| 明确文档修改 | 用户直接要求修改某个功能文档、某段说明、某个 topic/workflow | 定位目标后读取目标本地 Markdown 文件，按用户要求直接修改；仍需核对必要来源 |
| 根据最新代码更新文档 | 用户要求“根据最新代码更新文档”“同步当前实现”“文档跟代码对齐” | 定位文档 + 阅读代码 + 必要时用 git diff 提取关键机制变化，再更新对应 Topic/Workflow/Troubleshooting |
| 健康审计 / query 污染排查 | 用户要求体检、检查冲突、旧页面污染、errata/report 归位 | 先输出审计结果；中高风险写入按 Maintain Proposal 确认 |

明确文档修改和根据最新代码更新文档是用户已经授权写文档的场景，不需要为了普通文本更新额外输出 proposal。以下情况仍必须先 proposal 或人工确认：删除页面、删除业务规则、改主边界、active 改 deprecated、合并/拆分/重命名、多个来源冲突、代码与设计不一致且无法判断以谁为准、影响外部客户或接口行为的确定结论。

## 用户没有指定文件时的定位规则

用户没有指定文件时，不要先全仓 grep 文档目录。先按 `references/devwiki.md` 做结构化定位：

1. 提取用户内容中的功能名、模块名、接口、配置项、错误码、日志词、中文/英文同义词。
2. 执行 `wikimesh search index <query...> --project <project>`。
3. index 无有效命中时执行 `wikimesh search glossary <query...> --project <project>`。
4. glossary 仍无有效入口时，按语义执行 `wikimesh search topic <query...> --project <project>` 或 `wikimesh search workflow <query...> --project <project>`。
5. 对候选执行 `wikimesh read <topic|workflow> <slug> --view card --project <project>` 验证。
6. card 匹配后读取 core/explain 判断修改面；若 `active_source=local` 且任务是写入类，读取目标本地 Markdown 文件并修改。

`wikimesh search` 返回的是候选排序，不是真相源；结论必须回到真实 Wiki、raw 或已核对代码。

## 场景 1：直接针对功能提出修改

当用户直接针对功能提出修改时：

1. 如果用户给出具体文件、slug、标题或路径，直接读取该目标和必要来源。
2. 如果用户没有指定文件，按上面的结构化定位找到对应 Topic 或 Workflow。
3. 读取目标本地 Markdown 文件；因为本身就是修改文件，不受 query “禁止直接读 topic/workflow 文件”的限制。
4. 按用户要求修改目标文档，同时检查是否需要同步 `index.md`、`glossary.md`、`log.md`。
5. 如果修改只是表达、补充规则、补充实现说明或入口修正，直接落盘；如果触发高风险条件，输出 Maintain Proposal。

## 场景 2：根据最新代码更新文档

当用户要求根据最新代码更新文档时：

1. 仍然先按 `references/devwiki.md` 找到对应功能的 Topic / Workflow / Troubleshooting。
2. 阅读目标文档的 core/explain、frontmatter sources、关联 raw/source。
3. 阅读相关代码；已有 Workflow 代码锚点时，从锚点文件开始，不直接全仓搜索。
4. 如果要确认真实代码修改，先执行：

```bash
git status --short
git diff --stat
git diff
```

5. 从 `git diff` 中只抽取关键机制变化：功能行为、配置语义、接口/字段、状态读写、数据流/调用链、校验规则、错误处理、测试验证、兼容性影响。
6. 不把简单提示文案修改、注释调整、格式化、无行为变化的重命名写成机制变化；除非目标文档本来维护的就是该用户可见文案。
7. 功能行为和边界只有转换为产品语义后才写 Topic；代码入口、API path、接口字段、表名、内部配置字段、调用链、状态读写、实现差异、修改影响和测试验证写 Workflow；故障现象、日志和恢复路径写 Troubleshooting。

## 问题类型

| 类型 | 含义 | 典型处理 |
|---|---|---|
| 覆盖遗漏 | raw/source 有关键事实，Wiki 未覆盖 | 补充对应权威页面 |
| 过度摘要 | Wiki 太薄，导致关键规则、边界、异常丢失 | 按模板重写相关小节 |
| 无来源结论 | Wiki 写了结论，但找不到 raw/source/code 支持 | 标记待确认，不能编造 source |
| 证据冲突 | 多个 source 或 Wiki 之间描述不一致 | 输出冲突表，需确认后改 |
| 历史失效 | Wiki 内容曾经适用，但当前版本/实现不再适用 | 标记历史范围，更新当前结论 |
| 实现偏差 | 设计文档与代码实现不一致 | Topic 写产品语义下的功能结论，Workflow 写实现差异 |
| 差异报告误落盘 | maintain 把 errata/report 写成 active Wiki | 移到 outputs/report 或删除，结论合并回权威页 |
| 入口错误 | index/glossary 或页面链接指向错误或缺失 | 修正入口和页面链接 |
| 查询污染 | 旧页面或报告页被 query 命中并误导回答 | 降级、排除、改入口、更新索引 |
| 模板不合规 | 标题、frontmatter、来源、状态字段不符合规范 | 低风险可直接修 |

## 差异报告规则

Maintain 可以生成差异审计，但不得把差异审计长期写成 active Wiki 页面。

禁止创建或保留这类 active 页面：

```text
wiki/sources/*implementation-errata*.md
wiki/topics/*errata*.md
wiki/workflows/*errata*.md
wiki/*设计稿与实现差异*.md
```

除非用户明确要求保存审计报告，否则差异审计只输出在本次响应或 proposal 中。

如果用户要求保存报告，只能写入：

```text
wiki/outputs/<topic>-maintain-report-YYYY-MM-DD.md
```

并且必须使用：

```yaml
status: report
exclude_from_query: true
visibility: internal
```

报告顶部必须写：

```markdown
> 这是维护过程报告，不是功能事实来源。
> 当前功能规则以对应 Topic 为准。
> 当前实现路径以对应 Workflow 为准。
```

有效结论必须合并回权威页面：

| 差异类型 | 正确归位 |
|---|---|
| 能力边界差异 | Topic |
| 功能行为、规则、配置、边界差异 | Topic，但必须使用产品语义 |
| 代码入口、API path、接口字段、表名、内部配置字段、调用链、实现差异 | Workflow |
| 故障现象、日志、修复路径差异 | Troubleshooting |
| 维护过程对照表、审计表 | outputs/report，不进入 active query |

## Maintain Proposal

高风险、破坏性、冲突或归属不明时，先输出：

```markdown
# Maintain Proposal

## 维护范围

## 发现的问题

| 页面 | 问题类型 | 严重级别 | 说明 | 建议动作 |
|---|---|---|---|---|

## 差异归位计划

| 差异项 | 当前所在 | 目标页面 | 修改方式 | 是否需确认 |
|---|---|---|---|---|

## 证据与来源

## Query 污染风险

## 需要你确认的问题

## 不建议自动修改的内容
```

## 维护后验证

写入后按影响面验证：

```bash
wikimesh qmd collection update docs --project <project>
wikimesh qmd embed --project <project>
```

如果维护动作涉及 topic / workflow 的关系、重命名、拆分、合并、断链或入口修复，还必须执行：

```bash
wikimesh check document
wikimesh check graph
```

再次搜索原问题关键词，检查 qmd 是否优先命中权威页面，errata/report 是否不会成为 query 入口，旧页面是否明确标记历史/废弃，index/glossary 和页面入口链接是否指向当前机制，并用 2 到 5 个 seed question 测试 query 是否还会答旧内容。

## 禁止事项

- 不要修改 raw/source 原始资料。
- 不要未读 source 就修改 Wiki。
- 不要只看单页上下文就重写结论。
- 不要把不确定内容写成确定事实。
- 不要静默处理冲突。
- 不要跳过 proposal 直接做高风险改动。
- 不要删除页面，除非用户明确确认。
- 不要维护完 Wiki 却不更新 index/glossary 和页面入口链接。
- 不要维护完 Wiki 却不更新 qmd 索引。
- 不要把设计稿与实现差异长期写成 active Wiki 页面。
- 不要把 errata/report 放到 `wiki/sources/` 作为事实来源。
- 不要让 errata/report 成为 query 主入口。
- 不要在 Topic/Workflow 的 sources 中引用 maintain report 来支撑事实。
- 不要只写差异报告而不合并回权威页面。
- 不要在 index/glossary 或页面入口链接中指向 errata/report 作为当前结论。
- 不要编造 citation/source。
- 不要给无来源事实硬配来源。
- 不要让 outputs/report 参与正常 query。
