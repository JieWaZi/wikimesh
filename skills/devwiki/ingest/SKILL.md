---
name: "devwiki-ingest"
description: "当用户提供设计文档、需求文档、接口文档、配置说明、部署文档、测试文档、会议纪要、排障记录、代码逻辑片段或讨论结论，并要求消化、导入、生成 Wiki 或构建知识库时使用。"
argument-hint: "<文档路径、目录、文本片段或待摄入范围>"
---

# /devwiki-ingest

将一份或一批原始资料转成可维护知识。不要直接写散文总结；必须先分类、控粒度、检查已有 Wiki、输出 proposal，等用户在 proposal 之后显式确认后再落盘。

## 快速执行规则

1. 默认处于 `discussion_only`：用户要求“生成 Wiki / 导入资料 / 构建知识库”只表示启动 ingest 分析流程，不等于允许落盘。
2. 写入前必须先输出 Ingest Proposal；只有用户在 proposal 后明确回复“确认落盘”或“按 proposal 写入”，才进入 `confirmed_write`。
3. 不要从原始资料直接跳到最终页面；先抽取知识信号，再判断写入位置。
4. 写新页面前必须先检查 `wiki/index.md`、`wiki/glossary.md`、`wiki/topics/`、`wiki/workflows/`、`wiki/troubleshooting/`，避免重复建页。
5. 默认一个主题最多对应 1 个 Topic、1 个 Workflow；拆分、合并、重命名或改主关系前必须让用户确认。
6. Ingest 只做知识信号抽取、页面归属建议和任务编排；Topic / Workflow 的正文边界、模板细节和质量检查由对应页面 skill 负责。

一句话区分：

```text
Topic 是主题页：主题边界、功能规则、关键状态、关联 Workflow
Workflow 是工程实现页：代码入口、调用链、关键逻辑、修改影响
Troubleshooting 是排障页：故障现象、日志、诊断路径、恢复步骤
```

## Reference 读取策略

默认只读本文件，不要在任务开始时一次性读取所有 references。只有触发条件满足时再读取对应 reference：

| 触发条件 | 读取 |
|---|---|
| 写入、重命名、拆分、合并或改主关系前 | `references/mutation-safety.md` |
| 判断页面边界、事实来源或证据放置时 | `references/evidence-grounding.md` |
| 判断内容应该放入 card、core、explain 还是保留 raw 时 | `references/knowledge-placement.md` |
| 检查已有 Wiki、结构化搜索、读取关联 topic/workflow 时 | `references/devwiki.md` |
| 需要语义升档或向量检索时 | 继续按 `references/devwiki.md` 使用 `wikimesh qmd query`；需要向量检索前先执行或确认 `wikimesh qmd embed` |
| 需要结合当前代码、核对实现或写入代码定位时 | `references/code-tracing.md` |
| 更新 `wiki/index.md`、`wiki/glossary.md` 或 `wiki/log.md` 时 | `references/common-file-format.md` |
| 确认要生成 Topic 页面正文时 | 加载 `devwiki-topic` |
| 确认要生成 Workflow 页面正文时 | 加载 `devwiki-workflow` |

## 工作流程

### Phase 1：解析输入

1. 展开 `source` 范围，得到待处理资料列表。
2. 提取每份资料的标题、类型、版本、日期、owner、hash、关键术语。
3. 初步识别候选 Topic、Workflow、Troubleshooting。
4. 批量处理前，先抽样 3 到 5 份验证抽取质量、命名和页面粒度。

### Phase 2：抽取知识信号

```markdown
## 知识信号

### Topic 信号

- 主题目标：
- 所属 module：
- 功能边界：
- 核心行为：
- 关键规则：
- 关键配置 / 状态：
- 关联 workflow：
- glossary 候选关键术语：
- 建议写入重点：
- 建议不写入内容：
- 适合放入 explain 的内容：

### Workflow 信号

- 关联 topic：
- 入口点：
- 代码定位：
- 规则到实现映射：
- 修改影响：
- 测试验证：
- 调用链：
- 关键逻辑：
- 状态读写：
- 配置处理：
- 异常与恢复：
- glossary 候选关键术语：

### Troubleshooting 信号

- 现象：
- 日志 / 错误码：
- 诊断路径：
- 可能原因：
- 修复方式：

### 冲突与不确定内容

### 需要用户确认的问题
```

### Phase 3：检查已有知识

1. 先读取或搜索 `wiki/index.md`、`wiki/glossary.md`。
2. 再搜索 `wiki/topics/`、`wiki/workflows/`、`wiki/troubleshooting/`。
3. `wikimesh search index/glossary` 命中低置信、噪声过大或无法排序时，按 `references/devwiki.md` 升档到 `wikimesh search <topic|workflow> <query...>`；仍不足且问题属于概念、设计、意图或跨页面关系时，直接使用 `wikimesh qmd query <question>`。
4. 如果 `wikimesh search topic/workflow` / `qmd search` 报错、超时、collection 未注册或 cache 不可写，降级为 `wikimesh search index/glossary`，并在 proposal 中说明本轮 qmd 不可用。

### Phase 4：分类和控粒度

1. 一个主题默认最多对应 1 个 Topic、1 个 Workflow。
2. Troubleshooting 只在资料包含明确故障、日志、诊断或修复路径时生成。
3. 不要因为出现多个接口、多个字段、多个分支就拆成多个页面。
4. 只有用户明确要求、多个明显独立主题、运行时链路完全不同或单页会过长时，才建议拆分。
5. 拆分、合并或改主关系时，只在 proposal 中说明理由和风险；具体页面边界交给对应 Topic / Workflow skill 判断。

### Phase 5：生成任务

Ingest 是编排器，只生成 TopicTask / WorkflowTask / TroubleshootingTask，不写完整 Topic 或 Workflow 正文。

- Topic：加载 `devwiki-topic`，输入 TopicTask、来源、已有页面和放置建议
- Workflow：加载 `devwiki-workflow`，输入 WorkflowTask、关联 Topic 摘要、代码线索和放置建议
- Troubleshooting：使用本文件的简化模板

生成任务时：

1. 只给出该页面类型负责的内容范围。
2. 按 `references/knowledge-placement.md` 标注 `card/core/explain/raw` 放置建议。
3. TopicTask 必须带 `module` 建议；如果只能推断，标注置信度并放入 proposal 待确认问题。
4. TopicTask / WorkflowTask 必须列出 glossary 候选关键术语。
5. 先检查 `wiki/glossary.md` 是否已有关键术语或等价别名；不存在才添加。
6. 所有关键事实必须带来源。
7. 不确定内容写入“来源说明”或 proposal 的待确认问题。
8. TopicTask / WorkflowTask 只写任务范围和放置建议，不代替页面 skill 编写正文。

### Phase 6：输出 Ingest Proposal

写入前必须先输出 proposal。

```markdown
# Ingest Proposal

## 输入来源

## 拟写入文件

| 路径 | 类型 | 动作 | 风险等级 | 原因 | 置信度 |
|---|---|---|---|---|---|

## Topic 任务

| 目标 Topic | Module | 动作 | 输入来源 | 写入重点 | 不写入内容 |
|---|---|---|---|---|---|

## Workflow 任务

| 目标 Workflow | 动作 | 输入来源 | 写入重点 | 不写入内容 |
|---|---|---|---|---|

## View 分配建议

| 内容 | 目标页面 | 目标 View | 原因 |
|---|---|---|---|

## 入口和链接更新建议

## 术语更新建议

## 冲突与不确定内容

## 需要你确认的问题

## 暂不写入的内容

## 等待确认

如果以上路径、动作和内容摘要都认可，请明确回复“确认落盘”或“按 proposal 写入”。
```

### Phase 7：确认后落盘

只有在用户明确确认 Ingest Proposal 后，才进入 `confirmed_write` 并执行：

1. 创建或更新目标页面。
2. 更新 `wiki/index.md`。
3. 如果新建了 Topic 或 Workflow，必须更新 `wiki/glossary.md`：先查是否已有关键术语或等价别名，不存在才按 `references/common-file-format.md` 添加。
4. 追加 `wiki/log.md`。
5. 如果用户要求保存报告，写入 `wiki/outputs/<slug>.md`。
6. 执行或提示执行：

```bash
wikimesh qmd collection update docs --project <project>
wikimesh qmd embed --project <project>
```

7. 如果本次修改了 `wiki/topics/` 或 `wiki/workflows/`，必须执行：

```bash
wikimesh check document
wikimesh check graph
```

## Troubleshooting 简化模板

仅当资料包含明确故障、日志、诊断或修复路径时生成。

```markdown
---
title: ""
slug: ""
kind: troubleshooting
status: draft
summary: ""
topics: []
workflows: []
sources: []
visibility: internal
confidence: medium
last_verified_at: YYYY-MM-DD
---

# <排障主题>

## 现象

## 诊断路径

## 可能原因

## 修复 / 恢复

## 相关 Topic / Workflow

## 来源说明
```

## 落盘前检查

- 实际写入路径是否完全包含在 Ingest Proposal 的“拟写入文件”表内。
- Topic / Workflow 正文是否由对应页面 skill 处理，而不是由 ingest 直接展开。
- 是否没有因为字段、接口或分支过多而直接拆页。
- 新建 Topic 或 Workflow 后，是否已先查 `wiki/glossary.md` 并补充缺失的关键术语。
- 如果更新 `wiki/index.md`、`wiki/glossary.md` 或 `wiki/log.md`，是否已执行 `wikimesh check document` 校验格式。
