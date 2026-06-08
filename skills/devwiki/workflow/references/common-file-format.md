# DevWiki 通用文件格式

本文件统一定义 `wiki/index.md`、`wiki/glossary.md`、`wiki/log.md` 的格式。任何 Skill 更新这三个文件前都先读取本文件，不要在 Skill 正文中重复定义这些格式规则。

格式校验统一使用：

```bash
wikimesh check document
```

## 共同规则
- 不使用 YAML frontmatter。
- 不使用 `devwiki:section`

## `wiki/index.md`

用途：DevWiki 的人工可读入口目录，帮助 Agent 快速找到权威页面。

固定标题：

```markdown
# Wiki Index
```

必需结构：

```markdown
# Wiki Index

| type | description | slug |
|---|---|---|
| topic | <一句话说明> | <topic-slug> |
| workflow | <一句话说明> | <workflow-slug> |
| troubleshooting | <一句话说明> | <troubleshooting-slug> |
```

写作规则：

- 只放当前推荐入口和高价值导航。
- 每条入口必须写成表格行，字段顺序固定为 `type / description / slug`。
- `type` 可记录 `topic`、`workflow` 或 `troubleshooting`。
- `slug` 对 topic/workflow 必须是后续 `wikimesh read <type> <slug>` 可直接使用的页面 slug，不写文件名和 wiki link。
- `description` 写一句可检索、可让 agent 做语义判断的说明。
- 不写完整页面摘要、实现细节、排障过程或维护报告。
- 不把 deprecated、report、outputs 页面作为主入口。

## `wiki/glossary.md`

用途：保存全局术语、稳定入口概念和常用别名，帮助 query 消歧和路由。

固定标题：

```markdown
# Glossary
```

必需结构：

```markdown
# Glossary

| glossary | type | description | slug |
|---|---|---|---|
| <术语或别名> | topic | <一句话说明> | <topic-slug> |
```

术语选择规则：
- 优先沉淀业务能力、系统能力、跨页面主题、稳定领域概念和常用别名。
- 不要把单个进程、文件名、字段名、配置键、CSV 列、动作码、函数名、日志关键字或检索词当作 glossary 术语。
- 一个术语说明要回答“这个能力/主题解决什么问题、覆盖哪些关键行为、和哪些场景相关”。
- 每条术语必须写成表格行，字段顺序固定为 `glossary / type / description / slug`。
- `glossary` 写术语或常用别名；同一个概念有多个常用叫法时，可以各写一行并指向同一个 `slug`。
- `type` 可记录 `topic`、`workflow` 或 `troubleshooting`；v1 统一 CLI 的 `read/search` 类型只使用 `topic|workflow`。
- `slug` 对 topic/workflow 必须是后续 `wikimesh read <type> <slug>` 可直接使用的页面 slug，不写文件名和 wiki link。
- 新建 Topic 或 Workflow 后必须检查 `wiki/glossary.md`。
- 先搜索是否已有同名术语、等价别名或可复用入口；不存在才追加。
- 已有术语能覆盖时，不新增重复行；必要时只补充说明中的常用别名。

## `wiki/log.md`

用途：追加式记录 Wiki 维护动作，方便回溯写入历史。

固定标题：

```markdown
# Wiki Log

> Append-only chronological log.
```

追加格式：

```markdown
## YYYY-MM-DD

- <动作>：<路径或主题>；<一句话原因或影响>。
```

写作规则：

- 只追加，不重写历史记录。
- 每条记录写清动作、目标和原因。
- 不写长篇过程报告；维护过程报告写入 `wiki/outputs/`。
- 不把 log 当作功能事实来源。
