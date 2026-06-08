# DevWiki

面向研发场景的结构化 Wiki 与代码检索工作流。

DevWiki 的目标不是临时 RAG，而是把需求、设计、功能说明、测试资料和代码定位线索持续沉淀成可维护、可检索、可修正的知识层。

## 什么是 DevWiki

DevWiki 是一个单产品知识底座。文档库可以独立存在，并通过 `wikimesh repo link` 在用户级配置中关联一个或多个代码库。核心思路是：

- 把原始资料放在 `raw/`
- 把结构化知识沉淀到 `wiki/`
- 用 `topics` 合并能力边界与功能规则
- 用 Topic frontmatter 的 `module` 字段把相关主题聚合到 graph 父节点
- 用 `workflows` 说明工程入口、调用链、关键逻辑、修改影响和验证方式
- 用 `troubleshooting` 沉淀故障现象、诊断路径、证据和修复建议
- 用 `wikimesh qmd ...` 加速 `wiki` 结构化知识召回
- 按目标产物直接使用摄入、查询、维护、代码反向成文或 qmd 检索层维护命令

## 快速开始

### 第一步：执行 `wikimesh init`

```bash
wikimesh init "{{PROJECT_NAME}}" --agent {{AGENT}} --code-dir "{{PRIMARY_CODE_DIR}}" --yes
```

`wikimesh init` 会完成以下动作：

- 在当前目录下创建 `{{PROJECT_SLUG}}/` 作为 DevWiki 文档库根目录
- 在文档库目录生成 `README.md`
- 在文档库目录生成当前 agent 对应的运行时文件 `{{RUNTIME_FILE}}`
- 生成 `.wikimesh/qmd.yaml`
- 在文档库目录安装完整 DevWiki skills
- 在初始化结束后提示用户：如需，可手动执行 `wikimesh qmd download --root .`

### 第二步：同步 `wikimesh qmd` 检索层

```bash
wikimesh qmd sync --root .
wikimesh qmd sync --root . --apply
wikimesh qmd update
wikimesh qmd status
```

需要提前下载模型时：

```bash
wikimesh qmd model download all
```

### 第三步：准备原始资料

把原始文档放到 `raw/` 对应目录：

- `raw/requirements/`
- `raw/designs/`
- `raw/features/`
- `raw/tests/`

### 第四步：开始构建 Wiki

完成初始化后，就可以进入 Agent 运行时执行：

- `devwiki-ingest`
- `devwiki-maintain`
- `devwiki-code`
- `devwiki-query`
- `devwiki-code-to-doc`

## 典型使用闭环

### 从原始资料启动第一版 Wiki

1. 把需求、设计、功能说明、测试资料放入 `raw/`
2. 执行 `devwiki-ingest`
3. Agent 先输出 proposal，说明准备创建或更新哪些 topic、workflow、troubleshooting
4. 对拆分边界、命名冲突和中高风险写入做确认后落盘

### 查询功能、设计或代码位置

1. 用 `devwiki-query` 提出问题
2. Agent 用 `wikimesh search/read` 在 `topics / workflows` 中按意图召回证据；排障知识可作为页面内容和导航入口，但 v1 CLI 读取类型只使用 `topic|workflow`
3. 只有问题涉及当前实现、代码位置、修改影响或排障时，才继续核对代码

### 修改代码或开发功能

1. 在关联代码库中先判断目标产物和定位锚点
2. 如果用户只给出领域知识、功能名、特性名、业务规则、配置语义或接口行为，Agent 可建议显式使用 `$devwiki-code`
3. 显式使用 `$devwiki-code` 后，Agent 先从 DevWiki workflow 定位代码入口和规则边界，再核对当前代码
4. 如果用户已经给出具体文件、函数、代码块、当前 diff、完整 patch 或明确替换方式，按普通编辑任务处理，不自动进入 `devwiki-code`
5. 按测试、实现、验证的顺序修改当前代码仓；不默认写入 DevWiki 文档

### 从代码反向生成 Wiki 文档

1. 用 `devwiki-code-to-doc` 提供接口、文件、函数、路由、配置项或日志锚点
2. Agent 从真实代码追踪入口和关键边界，默认生成或更新 `workflows`
3. 只有需要补功能知识时，才同步更新 `topics`

## 核心能力

| 技能 | 作用 |
|------|------|
| `devwiki-ingest` | 增量消化需求、设计、测试或会议资料，生成 TopicTask / WorkflowTask、术语和入口导航 |
| `devwiki-topic` | 创建或维护 `wiki/topics/`，只写主题边界、功能规则、关键状态和关联 Workflow |
| `devwiki-workflow` | 创建或维护 `wiki/workflows/`，只写工程入口、代码定位、调用链、修改影响和验证方式 |
| `devwiki-maintain` | 维护已有 Wiki 的证据一致性、过期内容、引用缺失、入口错误和 query 污染 |
| `devwiki-code` | 显式 `$devwiki-code` 入口，处理缺少代码锚点的领域/功能/特性修改，基于 DevWiki workflow 定位并修改当前代码仓 |
| `devwiki-query` | 只读查询已有 Wiki、raw 和文档内代码线索，回答功能、设计、流程、代码定位和排障问题；缺少代码锚点的真实代码核查只建议显式 `$devwiki-code` |
| `devwiki-code-to-doc` | 从代码、接口、配置项、日志或路由反向生成或更新 workflow / 代码定位页面 |

## 目录结构

```text
./
├── README.md                    ← 项目说明、使用方式与工作流入口
├── {{RUNTIME_FILE}}             ← 当前 agent 对应的运行时规则副本
├── .wikimesh/
│   └── qmd.yaml                 ← `wikimesh qmd` collection 配置
├── raw/
│   ├── requirements/
│   ├── designs/
│   ├── features/
│   └── tests/
└── wiki/
    ├── index.md
    ├── glossary.md
    ├── log.md
    ├── topics/
    ├── workflows/
    ├── troubleshooting/
    └── outputs/
```

其中：

- `raw/` 是只读原始资料层
- `wiki/topics/` 是主题页，合并能力边界与功能规则
- Topic 的 `module` frontmatter 字段用于 graph 聚合展示，不对应独立模块页面
- `wiki/workflows/` 是工程定位页
- `wiki/troubleshooting/` 是排障知识页
- `wiki/outputs/` 保存 ingest / maintain / query / code-to-doc 报告
- `wiki/glossary.md` 保存术语表；新建 Topic 或 Workflow 后必须先查是否已有关键术语或等价别名，不存在才添加

## 常用维护命令

```bash
wikimesh read topic <slug> --view card --project <project>
wikimesh read topic <slug> --view core --project <project>
wikimesh read workflow <slug> --view core --project <project>
wikimesh search topic <query...> --project <project>
wikimesh search workflow <query...> --project <project>
wikimesh repo info
wikimesh repo info <project>
wikimesh repo use <project> <local|remote>
wikimesh search index <query...> --project <project>
wikimesh search glossary <query...> --project <project>
wikimesh search workflow <query...> --project <project>
wikimesh read workflow <slug> --view core --project <project>
wikimesh server --project <project> --host 0.0.0.0 --port 5697
wikimesh graph --project <project> --host 127.0.0.1 --port 5696
wikimesh check document
wikimesh check graph
wikimesh update  # 更新当前 Wikimesh 可执行文件
wikimesh tool reset --scope wiki --project-root .
wikimesh tool log --wiki-root wiki --message "init | note"
```

代码仓 `AGENTS.md` / `CLAUDE.md` 的 DevWiki link block 会写入 `DevWiki project`。如果无法从 link block 判断 project，可执行 `wikimesh repo info`；无参数时仅输出已配置 project 名称 JSON 数组。

`wikimesh repo info <project>` 默认输出 JSON，包含当前激活来源类型 `active_source`、已保存的本地/远端 `sources` 和所有关联代码仓 `code_repos[].path`。同一 project 同时配置本地与远端时，用 `wikimesh repo use <project> <local|remote>` 切换当前来源，不要通过重复 `repo add` 覆盖配置。

## 当前边界

当前版本优先保证单产品文档库与一个或多个代码库的关联场景可演示、可闭环。重点覆盖：

- 原始文档进入 `raw/`
- `devwiki-ingest` 从原始资料生成和更新结构化 Wiki
- `devwiki-topic` / `devwiki-workflow` 新建页面后必须检查并按需补充 `wiki/glossary.md`
- `devwiki-maintain` 维护已有 Wiki 的证据一致性、过期内容、引用缺失和 query 污染
- 显式 `$devwiki-code` 覆盖缺少代码锚点、需要 DevWiki workflow 定位入口和规则边界的代码修改、功能开发、修复和测试验证
- `devwiki-query` 覆盖只读知识查询、设计理解、文档内代码定位和排障检索；缺少代码锚点的真实代码核查只建议显式 `$devwiki-code`
- `devwiki-code-to-doc` 从代码反向补 workflow，必要时补 topic
- `wikimesh qmd sync/update/status` 维护 qmd collection 注册、索引和状态
