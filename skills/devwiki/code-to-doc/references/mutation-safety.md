# 变更安全约束

> 供任何会写入 `wiki/`、提出结构调整、或修改代码相关元数据的技能共享使用。

## 核心规则

默认写入模式为 `discussion_only`。在该模式下，只能读取、搜索、分析和输出 proposal，不得创建、修改或删除任何文件。

任何写入都必须先输出 proposal。所有写入，无论风险高低，都必须等用户在 proposal 之后显式确认，才能进入 `confirmed_write`。

用户要求“生成 / 导入 / 整理 / 更新 / 维护 Wiki”只表示进入分析和 proposal 流程，不等于落盘确认。用户沉默、继续讨论、补充资料、要求“继续分析”也不等于落盘确认。

只有用户在 proposal 之后明确回复类似以下内容时，才算确认：

- “确认落盘”
- “按 proposal 写入”
- “可以写入这些文件”
- “确认按上面的路径和动作修改”

## 写入模式

| 模式 | 触发条件 | 允许动作 |
|---|---|---|
| `discussion_only` | 默认模式；用户只是要求分析、整理、生成、导入、看看 | 读取、搜索、分析、输出 proposal；不得写文件 |
| `dry_run` | 用户要求先看会改什么、先给方案、dry-run | 输出拟写内容、路径清单、diff 风格预览；不得写文件 |
| `confirmed_write` | 用户在 proposal 之后明确确认落盘 | 只能按 proposal 中列出的路径和动作写入 |

## 风险分层

风险等级只影响 proposal 的详细程度，不影响是否需要确认。

### 低风险

- 追加确定性日志
- 刷新派生索引
- 刷新页面内联 `sources.hash`
- 更新明显过期的生成输出

低风险仍需 proposal 后确认；proposal 可以较短，但必须列出拟写路径和动作。

### 中风险

- 为已有 Topic 增加 `related_topics`
- 为已有 Workflow 追加辅助代码定位
- 为已有 Workflow 补充次级入口或测试验证
- 在不改变主题边界的前提下收紧过时 Topic 摘要
- 更新 Workflow 的调用链摘要

中风险 proposal 必须列出证据、影响范围、待确认问题和不会修改的内容。

### 高风险

- 新建 Topic
- 新建 Workflow
- 新建 Troubleshooting
- 合并或拆分 Topic
- 合并或拆分 Workflow
- 改变 Topic 与 Workflow 的主关联关系
- 替换 Workflow 的主代码定位
- 删除、重命名、降级或改变主入口页面

高风险 proposal 必须列出候选方案、推荐方案、风险、回退方式和需要用户拍板的问题。

## Proposal 内容要求

一个合格的 mutation proposal 至少应包含：

- 当前写入模式
- 要改什么
- 拟写入文件路径
- 每个路径的动作：create / update / append / delete
- 为什么这样改
- 证据是什么
- 风险等级
- 还有什么不确定
- 哪些内容暂不写入
- 如果用户确认，会发生什么

如果多个 topic 或 workflow 仍然都像候选，就不要自己拍板，应向用户提问。

## 落盘前检查

进入 `confirmed_write` 前必须确认：

- proposal 已经输出；
- 用户确认发生在 proposal 之后；
- 实际写入路径完全包含在 proposal 中；
- 实际写入动作完全匹配 proposal；
- 没有写入 proposal 未列出的文件；
- 没有把待确认问题写成确定事实。

## 不该做什么

- 不要为了省事把高风险动作降级成低风险。
- 不要把结构重写藏进一个小文案修改里。
- 不要静默改变 Topic 边界、Workflow 归属或 Topic-Workflow 主关系。
- 不要把用户沉默当确认。
- 不要把用户最初的“生成 / 导入 / 整理”请求当成落盘确认。
- 不要把 Topic 边界、Workflow 归属或 Workflow 拆分藏在自动修复里。
