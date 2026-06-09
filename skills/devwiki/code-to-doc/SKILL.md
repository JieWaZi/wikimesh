---
name: "devwiki-code-to-doc"
description: "当需要从真实代码、接口 URL、关键文件、关键函数、页面路径、路由、配置项或日志反向整理 DevWiki 文档时使用。"
argument-hint: "<功能名称、接口 URL、关键文件、关键函数、路由、配置项或日志关键字>"
---

# /devwiki-code-to-doc

## 使用前置

开始前先读取通用约束：

- `references/evidence-grounding.md`
- `references/devwiki.md`
- 涉及写入、重分类或破坏性操作时，再读 `references/mutation-safety.md`
- 涉及代码追踪、代码归因或实现核对时，再读 `references/code-tracing.md`
- 涉及 `wiki/index.md`、`wiki/glossary.md` 或 `wiki/log.md` 更新时，再读 `references/common-file-format.md`
- 需要语义升档时，先按 `references/devwiki.md` 判断是否进入第 3 档；需要时再加载 `references/query-rules.md` 使用 `wikimesh query`

生成或更新页面时，不在本 Skill 内重新定义模板，直接转交对应页面 Skill：

- Topic：加载 `devwiki-topic`
- Workflow：加载 `devwiki-workflow`

内容放置由对应页面 Skill 依据页面模板处理；本 Skill 只输出代码证据、边界和建议写入重点。

## 核心定位

Code-to-doc 不是模板写作 Skill，而是代码理解与证据追踪 Skill。
它也不是代码修改 Skill；如果用户目标是开发、修改、修复、重构、补测试或提交生产代码，且只给出领域目标、功能名、特性名、业务规则、配置语义或接口行为，只建议显式使用 `$devwiki-code`。

普通文档改写或用户已贴出完整文档片段和明确改法时，不自动转交 `devwiki-code`，按普通文档编辑任务处理。用户已经给出具体文件、函数、代码块、当前 diff、完整 patch 或明确替换方式时，也不自动转交 `devwiki-code`。

它负责回答：

- 从哪个代码入口开始；
- 如何一级一级向下追踪；
- 追到什么深度可以停止；
- 哪些代码证据可以支撑 Wiki；
- 哪些结论应该进入 Workflow；
- 哪些功能语义、能力边界或规则需要同步到 Topic；
- 哪些异常、日志、恢复路径需要同步到 Troubleshooting；
- 当代码、raw、wiki 冲突时如何处理；
- 什么时候必须向用户提问。

## 默认产出

默认产出优先级：

1. 默认写入 `wiki/workflows/<slug>.md`
   - 代码入口；
   - 调用链；
   - 关键逻辑；
   - 状态读写；
   - 配置处理；
   - 异常与恢复实现；
   - 修改影响；
   - 测试验证。

2. 必要时更新 `wiki/topics/<slug>.md`
   - 仅当代码追踪能够确认产品语义下的功能行为、配置语义、联动或边界时同步更新。
   - 代码事实必须先转成产品表达，再进入 Topic。
   - Topic 不写代码引用，只链接 Workflow。
   - Topic 的 sources 不写代码文件路径或 `kind: code`。

3. 必要时更新 `wiki/troubleshooting/<slug>.md`
   - 仅当输入锚点是日志、错误码、异常现象，或代码追踪确认了诊断/恢复路径时更新。

## 输入锚点

用户至少应提供一个锚点：

- 功能名称；
- API URL；
- 路由；
- 页面路径；
- 关键文件；
- 关键函数；
- 配置项；
- 日志关键字；
- 错误码；
- 已知 Topic / Workflow slug。

如果只有功能名称，不要马上提问。先自主搜索 wiki、raw、代码。只有多轮搜索后仍无法稳定定位，才向用户提问。

## 来源优先级

默认按以下顺序调查：

1. `wiki/`
   - 先看已有 topic、workflow、troubleshooting，避免重复整理。
2. `raw/`
   - 再看需求、设计、功能说明、接口说明、测试资料，理解历史意图。
3. 本地代码
   - 用当前代码确认真实实现，并纠正文档漂移。
4. 用户澄清
   - 只有无法继续确认或必须用户拍板时才提问。

关键原则：

- `wiki/` 和 `raw/` 是线索与历史，不是最终实现真相。
- 当前实现结论必须由当前代码支撑。
- 如果代码与 wiki/raw 冲突，必须在 proposal 或 Workflow 的“设计与实现差异 / 来源说明”中明确写出。
- 不得把历史设计默认当成当前实现。

## 归属判断

| 代码追踪发现 | 写入位置 |
|---|---|
| 入口、调用链、类、模块、函数、handler | Workflow |
| 状态读写、配置读取、校验、同步、下发 | Workflow |
| 异常、失败、重试、回滚、恢复实现 | Workflow |
| 代码与 raw/wiki 的实现差异 | Workflow |
| 修改影响、测试引用、验证建议 | Workflow |
| 能从代码确认且能用用户视角表达的功能行为、配置语义、边界规则 | Topic |
| 能从代码确认且能用用户视角表达的功能联动、异常边界 | Topic |
| 日志、错误码、诊断路径、修复/恢复路径 | Troubleshooting |

写入 Topic 前必须先判断这条代码结论能否用用户视角表达。能表达为能力、规则、状态、边界或结果的，才建议同步到 Topic；只能依赖内部名称、字段、表、接口或实现动作才能说明的，只写入 Workflow。

## 冲突与提问边界

如果代码、wiki、raw 冲突：

1. 不静默选择一方。
2. 不直接修改 raw。
3. 当前实现结论必须由当前代码支撑。
4. 产品设计或历史意图必须标明来源。
5. 冲突必须写入 proposal 的“冲突与不确定内容”。

必须提问的场景：

- 多个入口同样像，无法判断主链路；
- 用户提供的 URL、函数、路由在本地找不到；
- 动态注册、反射、配置下发导致静态追踪断裂；
- 需要新建、拆分、合并、重命名页面；
- 代码、wiki、raw 冲突，无法判断应该更新哪一层。

## 写入 Proposal

落盘前必须按 `references/mutation-safety.md` 输出 proposal。

```markdown
# Code-to-Doc Proposal

## 输入锚点

## 已检查资料

| 来源 | 结果 |
|---|---|

## 代码追踪摘要

| 层级 | 发现 | 证据 |
|---|---|---|

## 拟写入文件

| 路径 | 动作 | 写入重点 | 风险 |
|---|---|---|---|

## Topic 同步建议

Topic 同步建议必须说明哪些代码结论可以用用户视角表达，哪些内容应保留在 Workflow 不进入 Topic。

## Workflow 写入建议

## 术语更新建议

## 冲突与不确定内容

## 需要你确认的问题
```

## 确认后落盘

只有用户明确确认 Code-to-Doc Proposal 后，才进入 `confirmed_write`：

1. 创建或更新 `wiki/workflows/<slug>.md`。
2. 必要时更新 `wiki/topics/<slug>.md`。
3. 必要时更新 `wiki/troubleshooting/<slug>.md`。
4. 更新 `wiki/index.md`。
5. 新建 Topic 或 Workflow 后必须先查 `wiki/glossary.md` 是否已有关键术语或等价别名；不存在才按 `references/common-file-format.md` 添加。
6. 追加 `wiki/log.md`。
7. 执行：

```bash
wikimesh check document
wikimesh check graph
# 在文档库本地 source 根目录执行
wikimesh qmd collection update docs
wikimesh qmd embed
```

## 禁止事项

- 不要把代码引用写进 Topic。
- 不要把代码文件路径、函数名、类名、API path、接口字段、数据库表名、内部配置字段或 `kind: code` 写进 Topic。
- 不要把内部流程、内部状态读写、内部调度或其他实现层表达写进 Topic；必须提及时只表达用户可感知的行为或结果。
- 不要把代码引用写进 Topic 的 `sources`。
- 不要复制 Topic 的完整业务说明到 Workflow。
- 不要因为出现多个接口、多个 helper、多个分支就拆多个 workflow。
