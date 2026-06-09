---
name: "devwiki-topic"
description: "当已确认需要创建或维护 DevWiki Topic 页面，或需要整理主题边界、功能机制、关键规则、关键状态和关联 Workflow 时使用。"
argument-hint: "<TopicTask、topic slug、原始资料范围或已有 topic 路径>"
---

# /devwiki-topic

## 定位

`devwiki-topic` 只负责生成和维护 Topic。

Topic 是主题页，用来回答：

- 这个主题解决什么问题；
- 产品语义下的功能边界和核心机制是什么；
- 哪些产品规则、用户可见配置、状态会影响行为；
- 哪些边界、误区和联动需要注意；
- 需要看实现时应该跳转哪个 Workflow。

Topic 不负责代码实现，也不负责测试设计。代码路径、函数名、handler、API path、接口字段、表名、内部配置字段、调用链、状态读写实现、修改影响、测试位置和测试方法必须写入其他页面或由 Workflow/代码追踪处理。

一句话边界：

```text
Topic 解释产品语义下的功能机制和规则；Workflow 解释代码如何实现；Troubleshooting 解释故障如何处理。
```

## 输入

常见输入：

1. `devwiki-ingest` 输出的 TopicTask；
2. raw 资料、设计片段、产品帮助文案或用户补充说明；
3. 已有 `wiki/topics/<slug>.md`；
4. 关联 Workflow 的 card 摘要；
5. 需要同步的 glossary/index 入口。

如果没有 TopicTask，也可以从用户给出的主题、资料或已有页面开始，但写入前仍必须按 `references/mutation-safety.md` 输出 proposal 并等待确认。

## 使用前置

开始前按需读取：

- `references/topic_template.md`：创建或重写 Topic 时必须读取；
- `references/evidence-grounding.md`：判断事实来源、推断和证据放置时读取；
- `references/knowledge-placement.md`：判断内容应该进入 card、core、explain 还是保留 raw 时读取；
- `references/mutation-safety.md`：任何写入、重命名、拆分、合并或关系调整前读取；
- `references/devwiki.md`：本地 Wiki 命中低置信、需要结构化搜索或读取关联页面时读取。
- `references/common-file-format.md`：新建 Topic 后检查或更新 `wiki/glossary.md` 时读取。

不要读取 `workflow_template.md`。如果任务需要写 Workflow 正文，转交或加载 `devwiki-workflow`。

关联 Workflow 只允许读取 `card` 摘要来确认链接关系；不要为了写 Topic 去读取 Workflow 的 `code` 或 `explain`，避免把实现细节带入 Topic。

## 输出

默认输出：

```text
wiki/topics/<slug>.md
```

必要时附带：

- `wiki/index.md` 入口更新建议；
- `wiki/glossary.md` 术语更新建议；
- 关联 Workflow 的链接更新建议；
- `wiki/log.md` 追加内容。

## 核心规则
1. 只写主题边界、功能机制、关键规则、关键配置和状态。
2. Topic `core` 是产品语义层，不写实现摘要。
3. 不写代码路径、文件名、函数名、类名、handler、调用链、状态读写实现和修改影响。
4. 不写 API path、HTTP 方法、接口字段、数据库表名、数据库字段、ORM model、内部配置字段、内部枚举和内部常量。
5. 不写服务内部动作、内部运行时流程、内部持久化细节和内部调度机制；必须提及时只表达用户可感知的行为或结果。
6. 不写测试位置、测试方法、测试用例、验收清单或回归建议。
7. 不写 UI 字段清单、接口字段清单、完整操作列表或产品帮助全文。
8. 实现相关内容只写关联 Workflow 链接和一句话入口说明，不写代码路径。
9. 低频、低价值、高体积内容不进入 Wiki，保留 raw。
10. 不复制 raw 原文，不为了填模板强行写空内容；没有独立决策价值的小节可以省略。
11. 关系字段使用字符串数组，不维护 relation/reason 对象。
12. 页面必须包含 `card`、`core`、`explain` 三个 `devwiki:section`。
13. Topic frontmatter 必须尽量填写 `module`，表示所属稳定模块 slug；`module` 只作为 graph 父节点，不创建独立模块页面。
14. 新建 Topic 后必须检查 `wiki/glossary.md`；先确认 glossary 是否已有关键术语或等价别名，不存在才添加。

## Topic 表达边界

如果代码或实现证据确认了功能行为，需要写入 Topic，只能表达用户可感知的能力、规则、状态、边界或结果。不要把实现证据中的内部名称、内部动作、字段、表、接口或调用链直接搬进 Topic。

如果一条内容无法脱离实现名词才能说清楚，写入 Workflow，不写入 Topic。

## Topic 粒度规则

Topic 粒度按用户意图和产品语义划分，不按代码模块、接口、字段或函数划分。

允许大能力 Topic 与小能力 Topic 共存：

- 父 Topic 只写能力族边界、主对象语义和关联能力地图。
- 子 Topic 只写独立用户意图、独立规则和与父能力的边界。
- 父 Topic 只用一句话链接子 Topic，不展开子 Topic 规则。
- 子 Topic 只用一句话说明父 Topic 边界，不复制父 Topic 背景。
- 小能力无法独立用产品语言解释时，不拆 Topic。

拆 Topic 前必须判断：

- 是否有独立用户问题；
- 是否有独立产品规则或状态；
- 是否会显著减轻父 Topic `core`；
- 是否会增加候选噪声；
- 是否能用短 `card` 与父 Topic 区分。

## 知识放置规则

Topic 编写时先判断知识的查询频率、决策价值、稳定性和体积成本。

| 内容类型 | 放置位置 |
|---|---|
| 命中意图、关键边界、关联入口 | `card` |
| 产品语义下的主题摘要、功能边界、核心机制、关键规则、关键配置与状态 | `core` |
| 典型场景、边界误区、低频规则补充、来源与待确认 | `explain` |
| 接口细节、字段清单、表名、代码路径、完整操作步骤、测试设计、代码细节 | 不进入 Topic |
| 低频、低价值、高体积原文 | `raw` |

## 写入流程

1. 明确 TopicTask 或目标 topic slug。
2. 检查 `wiki/index.md`、`wiki/glossary.md`、`wiki/topics/`，避免重复建页。
3. 确认或推断该 Topic 的 `module`；若资料不足，在 proposal 的待确认问题中说明。
4. 读取相关 raw 和已有 topic。
5. 如有关联 Workflow，只读取其 card 摘要，不复制实现说明。
6. 读取 `references/topic_template.md`。
7. 按 `references/knowledge-placement.md` 分配 card/core/explain/raw。
8. 输出 Topic Proposal，列出拟写路径、动作、来源、风险、module 和待确认问题。
9. 用户明确确认后写入。
10. 如果是新建 Topic，读取 `references/common-file-format.md`，检查 `wiki/glossary.md` 是否已有关键术语或等价别名；不存在才追加术语说明。
11. 执行：

```bash
wikimesh check document
wikimesh check graph
# 在文档库本地 source 根目录执行
wikimesh qmd collection update docs
wikimesh qmd embed
```

## Topic Proposal

```markdown
# Topic Proposal

## 目标 Topic

## 输入来源

## 已检查页面

## 拟写入文件

| 路径 | 动作 | 写入重点 | 不写入内容 | 风险 |
|---|---|---|---|---|

## 关联 Workflow

## 所属 Module

## 内容放置判断

| 内容 | 放置位置 | 原因 |
|---|---|---|

## 入口和术语更新建议

## 冲突与不确定内容

## 需要你确认的问题
```

## 质量检查

- 是否没有代码路径、文件名、函数名、类名、handler、API path、接口字段、表名、内部配置字段、调用链。
- 是否没有服务内部动作、内部运行时流程、内部持久化细节和内部调度机制；确需表达时是否只写了用户可感知的行为或结果。
- 是否没有测试设计、验收清单、UI 字段清单或接口字段清单。
- 是否能用短 card 判断命中，且 card 没有核心规则摘要、字段清单或实现链路。
- core 是否覆盖产品语义下的主题边界、核心机制、关键规则、关键配置和状态。
- 功能边界是否只写真实相邻主题，未写泛化排除项。
- 父子 Topic 是否职责清楚，没有复制彼此核心规则。
- explain 是否只是 core 的低频补充，而不是详细说明或产品帮助文档，是否控制篇幅，没有为了完整而堆内容。
- module 是否为稳定模块 slug，且没有创建独立模块页面。
- workflows / related_topics 是否是字符串数组。
- 新建 Topic 后是否已先查 glossary，确认关键术语存在或已补充。
- 来源、冲突和不确定内容是否清楚标注。
- 是否已执行 `wikimesh check document` 校验 index/glossary/log 格式和 `devwiki:section` 分块。
- 是否已执行 `wikimesh check graph`。
