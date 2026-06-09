---
name: "devwiki-workflow"
description: "当已确认需要创建或维护 DevWiki Workflow 页面，或需要整理工程入口、代码定位、调用链、修改影响和验证方式时使用。"
argument-hint: "<WorkflowTask、workflow slug、代码锚点、接口、函数、配置项或已有 workflow 路径>"
---

# /devwiki-workflow

## 定位

`devwiki-workflow` 只负责生成和维护 Workflow。

Workflow 是工程实现页，用来回答：

- 代码入口在哪里；
- 规则如何映射到实现；
- 调用链、状态读写、配置处理和异常路径是什么；
- 修改会影响哪里；
- 实现层验证关注点是什么。

Workflow 不复制 Topic 的完整业务说明。功能边界、产品规则和主题背景应链接 Topic。

## 输入

常见输入：

1. `devwiki-ingest` 输出的 WorkflowTask；
2. `devwiki-code-to-doc` 的代码追踪摘要；
3. 关联 Topic 的 card/core 摘要；
4. 代码检索结果、接口 URL、关键文件、函数、配置项、日志关键字；
5. 已有 `wiki/workflows/<slug>.md`；
6. raw 资料或用户补充说明。

如果没有 WorkflowTask，也可以从代码锚点或用户问题开始，但写入前仍必须按 `references/mutation-safety.md` 输出 proposal 并等待确认。

## 使用前置

开始前按需读取：

- `references/workflow_template.md`：创建或重写 Workflow 时必须读取；
- `references/code-tracing.md`：需要代码追踪、代码归因或实现核对时读取；
- `references/evidence-grounding.md`：判断事实来源、推断和证据放置时读取；
- `references/knowledge-placement.md`：判断内容应该进入 card、core、explain 还是保留 raw 时读取；
- `references/mutation-safety.md`：任何写入、重命名、拆分、合并或关系调整前读取；
- `references/devwiki.md`：本地 Wiki 命中低置信、需要结构化搜索或读取关联页面时读取。
- `references/common-file-format.md`：新建 Workflow 后检查或更新 `wiki/glossary.md` 时读取。

不要读取 `topic_template.md`。如果任务需要写 Topic 正文，转交或加载 `devwiki-topic`。

## 输出

默认输出：

```text
wiki/workflows/<slug>.md
```

必要时附带：

- 关联 Topic 的 `workflows` 字段更新建议；
- `wiki/index.md` 入口更新建议；
- `wiki/glossary.md` 术语更新建议；
- `wiki/log.md` 追加内容。

## 核心规则

1. 只写工程实现知识。
2. 不复制 Topic 的完整功能说明；业务背景和产品规则只链接 Topic。
3. Workflow 必须关联至少一个 Topic。
4. 代码路径、函数、类、配置文件必须有证据。
5. 未核对代码不得写成确定事实。
6. 代码定位以文件 `path` 为粒度，一个文件一行。
7. 关键入口最多列 8 个，只列关键文件、关键入口和关键逻辑点，不列全量方法。
8. 代码定位与关键逻辑统一写入 `core` section，不写入 frontmatter。
9. 状态流转、配置处理、异常恢复、并发时序和实现差异进入 `explain`。
10. 低频、低价值、高体积内容不进入 Wiki，保留 raw。
11. 页面必须包含 `card`、`core`、`explain` 三个 `devwiki:section`。
12. 新建 Workflow 后必须检查 `wiki/glossary.md`；先确认 glossary 是否已有关键术语或等价别名，不存在才添加。

## Card 约束

Workflow `card` 只用于判断是否命中这条实现链路，不写完整入口表、调用链、修改影响或测试方式。

`card` 只保留：

- 命中意图：一句话说明该 Workflow 解释哪类实现链路；
- 关键边界：只写容易混淆的相邻 Workflow/Topic，没有真实歧义可省略；
- 支撑 Topic：链接对应 Topic。

实现入口、关键文件、修改影响和证据状态写入 `core`；主流程、状态变化、副作用和异常分支写入 `explain`。

## 写入流程

1. 明确 WorkflowTask、目标 workflow slug 或代码锚点。
2. 检查 `wiki/workflows/` 和关联 `wiki/topics/`，避免重复建页。
3. 读取关联 Topic 的 card/core 摘要，确认业务规则来源。
4. 按 `references/code-tracing.md` 核对真实代码入口和边界。
5. 读取 `references/workflow_template.md`。
6. 按 `references/knowledge-placement.md` 分配 card/core/explain/raw。
7. 输出 Workflow Proposal，列出拟写路径、动作、代码证据、风险和待确认问题。
8. 用户明确确认后写入。
9. 如果是新建 Workflow，读取 `references/common-file-format.md`，检查 `wiki/glossary.md` 是否已有关键术语或等价别名；不存在才追加术语说明。
10. 执行：

```bash
wikimesh check document
wikimesh check graph
# 在文档库本地 source 根目录执行
wikimesh qmd collection update docs
wikimesh qmd embed
```

## Workflow Proposal

```markdown
# Workflow Proposal

## 目标 Workflow

## 关联 Topic

## 输入锚点

## 代码追踪摘要

| 层级 | 发现 | 证据 |
|---|---|---|

## 拟写入文件

| 路径 | 动作 | 写入重点 | 不写入内容 | 风险 |
|---|---|---|---|---|

## Topic 反向链接更新建议

## 术语更新建议

## 冲突与不确定内容

## 需要你确认的问题
```

## 质量检查

- 是否关联至少一个 Topic。
- 是否没有复制 Topic 的完整功能背景。
- card 是否只用于命中判断，未写完整入口表、调用链或修改影响。
- 代码路径、函数、类、配置文件是否都有证据。
- 代码定位是否按文件归并，一个文件一行。
- 是否只列关键文件、关键入口和关键逻辑点，没有铺开辅助函数、普通校验或顺手读到的方法。
- 未核对内容是否标为待确认。
- 修改影响和测试验证是否可执行。
- 新建 Workflow 后是否已先查 glossary，确认关键术语存在或已补充。
- 是否已执行 `wikimesh check document` 校验 index/glossary/log 格式和 `devwiki:section` 分块。
- 是否已执行 `wikimesh check graph`。
