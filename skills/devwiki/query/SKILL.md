---
name: "devwiki-query"
description: "当用户询问项目能力、功能说明、配置规则、现有实现位置、排障知识、对外说明、比较分析或已有 DevWiki 知识，并且不要求本轮修改代码时使用。若用户要求修改当前代码仓且缺少明确代码锚点，需要 DevWiki 辅助定位入口和规则边界时，只建议显式使用 $devwiki-code。"
argument-hint: "<问题>"
---

# /devwiki-query

## 核心约束

- **证据落地**：每个关键结论必须落到真实来源（wiki 页面、raw 文件或文档内已记录的代码线索）
- **文档优先**：本 Skill 默认完整消费 DevWiki 文档后再回答。实现类问题优先读 workflow card → workflow core → workflow explain；不因为用户问“怎么实现”就自动搜索真实代码。
- **视图分层读取**：先用 `--view card` 判断命中，再用 `--view core` 回答主问题，只有 core 不够时读 `--view explain`。card 只放判断“是不是这个页面”的信息，core 放高频主结论，explain 放低频补充。
- **按需加载参考文档**：
  - 查询或读取 DevWiki 项目知识前 → 读 `references/devwiki.md`
  - 本地搜索低置信、需语义 query 升档时 → 先读 `references/query-rules.md`，再使用 `wikimesh query`

不要凭空回答项目事实；先查 DevWiki 文档，优先基于 topic / workflow / troubleshooting / raw 总结；每个关键结论都要能追溯来源。

没有可靠候选、没有独立 Topic/Workflow 或用户未确认候选时，只能回答“当前 Project Brain 没有足够信息支持该结论。”并列出知识缺口；不得把多个分散页面里的零散命中综合成一个新的能力定义、边界说明或推荐口径。不要输出“它更像是”“可以先按这个口径理解”“如果要作为新能力来描述”这类推断性补全。

## 与 devwiki-code 的边界

- 本 Skill 是只读查询：回答知识、规则、文档中的实现说明、代码线索、影响面和排障线索。
- query 不自动核对真实代码，也不执行 `rg` 代码搜索。
- 如果用户表达了要修改当前代码仓，且只给出领域知识、功能名、特性名、业务规则、配置语义或接口行为，缺少明确代码锚点，本 Skill 只建议显式使用 `$devwiki-code`，不自动转交。
- 如果用户已经给出具体文件、代码块、当前 diff 或明确改法时，不转交 `devwiki-code`；这属于普通代码编辑任务。
- 如果用户明确要求查代码、核对当前实现、找具体文件/函数/行号、确认真实调用链或运行态，本 Skill 不直接搜索代码；缺少代码锚点时只建议显式使用 `$devwiki-code` 做当前代码核查。
- 不要在需要修改当前代码且缺少代码锚点的请求中继续执行 query 的 locate_code 全流程；这类请求建议用户显式使用 `$devwiki-code` 走 `index → workflow card/core → 必要 topic core → 代码核对 → 测试 → 实现 → 验证`。
- 只有用户明确要求保存回答、沉淀结论或写入报告，且 `repo info <project>` 显示 `active_source=local` 时，本 Skill 才写 `wiki/outputs/`；`active_source=remote` 时只输出报告正文。

## Outputs

- 先给出语义识别结果：explain_topic / locate_code / troubleshoot / public_answer / compare
- 命中的 Topic / Workflow / Troubleshooting 页面
- 知识缺口、冲突和待确认项
- 必要的代码定位线索、修改影响和测试建议，只使用 DevWiki 文档中已有信息；缺少代码锚点且需要 DevWiki 定位入口时，建议显式使用 `$devwiki-code`，已有明确锚点时按普通代码查看处理
- 只有用户明确要求保存回答、沉淀结论、写入报告，且 `active_source=local` 时，才写入 `wiki/outputs/<query-slug>.md` 并追加 `wiki/log.md`；`active_source=remote` 时只输出报告正文和建议保存位置

## Workflow

### Step 1: 意图识别

1. 先读 `references/devwiki.md`，按其中的统一项目配置确定 project。
2. 如果用户要求修改当前代码仓，且缺少具体文件、函数、代码块、当前 diff、完整 patch 或明确替换方式，停止本 Skill，建议显式使用 `$devwiki-code`。
3. 如果用户已经给出具体代码锚点或明确改法，停止本 Skill，按普通代码编辑任务处理。
4. 判断问题语义：
   - `explain_topic`：用户想了解能力、功能、配置、边界、规则、联动。
   - `locate_code`：用户想基于 DevWiki 文档了解代码入口、调用链、接口、数据流、实现机制、影响面、测试入口，但不要求本轮修改代码，也没有明确要求核对当前代码仓。
   - `troubleshoot`：用户想解决报错、不生效、异常现象、诊断路径、修复建议。
   - `public_answer`：用户要求对外说明、客户口径、官网/文档口径。
   - `compare`：用户要求比较两个主题、方案或实现路径。

### Step 2: 定位和读取

按 `references/devwiki.md` 的结构化定位规则串行搜索：

```text
glossary keywords → index → glossary → topic/workflow → 必要时加载 query-rules.md 后使用 wikimesh query
```

- 定位和读取必须服从 `references/devwiki.md` 的 Glossary Alignment、Competitor Check、Card Scoring、Evidence Path 和 Confirmation Actions。
- 命中候选后先读 card。“确认匹配”不是 agent 自行判断通过；当 reference 判定为 medium / low 时必须停止在 card 阶段，并请求用户确认问题意图和业务范围，不得继续读取 core/explain。
- 只有 Evidence Path 为 high，或用户确认 medium 路径后，才按语义深度阅读 core/explain。
- locate_code 默认回答 Workflow 文档中的实现入口、模块职责、状态流/数据流、副作用和文档中已记录的代码线索。
- 没有可靠候选、没有独立 Topic/Workflow 或用户未确认候选时，不进入 raw/qmd/其他页面综合推断新主题；只说明未找到可靠依据、列出检索过的入口和建议用户补充锚点。
- 如果 `wiki/index.md`、`wiki/glossary.md` 或目标目录缺失，或者真实页面仍不足以支撑结论，输出：

```text
当前 Project Brain 没有足够信息支持该结论。
```

并建议给出更明确的指示和关键字

### Step 3: 显式代码核查建议

以下情况停止 query 的代码搜索动作，并建议显式使用 `$devwiki-code`：

- 只有用户明确要求查代码、核对当前实现、找文件函数或行号；
- 用户要求确认真实调用链、日志出处、配置读取点、测试入口或运行态；
- 用户要求修改建议且需要确认当前代码约束；
- wiki / raw / workflow 证据互相冲突，或文档不足以支撑当前代码事实；
- 排障问题必须确认真实日志、运行态或调用路径。

交接回答应说明：

- 已基于 DevWiki 文档确认了哪些入口、模块职责、状态流/数据流或副作用；
- 哪些结论仍需要当前代码核查；
- 缺少代码锚点且需要 DevWiki 定位入口时，建议显式使用 `$devwiki-code` 继续，并让 `devwiki-code` 按 workflow 锚点定向搜索代码；
- 已有具体文件、函数、代码块、当前 diff、完整 patch 或明确替换方式时，按普通代码查看或编辑任务处理，不转 `devwiki-code`；
- 如果没有查代码，明确说明：本轮基于 DevWiki 文档总结，未展开当前代码核查。

### Step 4: 按语义组织回答

每次回答先给一句语义识别：

```text
语义识别：explain_topic / locate_code / troubleshoot / public_answer / compare
```

explain_topic 使用 Topic 回答重点：主题解决什么问题、核心行为和关键规则、功能边界、关键配置与状态、需要看实现时进入哪个 Workflow。

explain_topic 必须以已确认的 Topic 为主依据；如果没有命中独立 Topic 或用户没有确认候选，不能把多个无关 Topic/Workflow 中的同词命中拼成“新能力说明”，只能说明当前没有相关依据和知识缺口。

locate_code 使用 Workflow 回答重点：Workflow 文档描述的实现入口、主要模块职责、状态流 / 数据流 / 副作用、文档中已记录的代码线索、Topic 规则如何映射到实现、修改影响是什么（仅限文档已记录）、如何测试验证（仅限文档已记录）。

troubleshoot 使用 Troubleshooting 回答重点：现象、诊断路径、可能原因、修复 / 恢复、相关 Topic / Workflow。

public_answer 回答重点：对外结论、可公开范围、不应展开的内部信息、依据。

compare 回答重点：比较对象、相同点、差异点、适用场景、依据 / 待确认。

### Step 5: 按需沉淀答案

只有用户明确要求保存回答、沉淀结论、写入报告，且 `repo info <project>` 显示 `active_source=local` 时，才创建 `wiki/outputs/<query-slug>.md` 并追加 `wiki/log.md`。如果 `active_source=remote`，只输出报告正文和建议保存位置。
