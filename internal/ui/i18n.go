package ui

// Catalog 集中保存 CLI 可见文案，避免命令实现里散落展示字符串。
type Catalog struct {
	// RootShort 是根命令的一句话说明。
	RootShort string
	// QMDShort 是 qmd 子命令分组说明。
	QMDShort string
	// CollectionShort 是 qmd collection 分组说明。
	CollectionShort string
	// CollectionAddShort 是 collection add 命令说明。
	CollectionAddShort string
	// CollectionListShort 是 collection list 命令说明。
	CollectionListShort string
	// CollectionRemoveShort 是 collection remove 命令说明。
	CollectionRemoveShort string
	// CollectionUpdateShort 是 collection update 命令说明。
	CollectionUpdateShort string
	// QMDStatusShort 是 qmd status 命令说明。
	QMDStatusShort string
	// QMDUpdateShort 是 qmd update 命令说明。
	QMDUpdateShort string
	// SearchShort 是 qmd search 命令说明。
	SearchShort string
	// VSearchShort 是 qmd vsearch 命令说明。
	VSearchShort string
	// QueryShort 是 qmd query 命令说明。
	QueryShort string
	// EmbedShort 是 qmd embed 命令说明。
	EmbedShort string
	// ModelShort 是 qmd model 分组说明。
	ModelShort string
	// ModelDownloadShort 是 model download 命令说明。
	ModelDownloadShort string
	// ModelLibShort 是 model lib 分组说明。
	ModelLibShort string
	// ModelLibInstallShort 是 model lib install 命令说明。
	ModelLibInstallShort string
	// WikiInitShort 是 init 命令说明。
	WikiInitShort string
	// WikiUpdateShort 是 update 命令说明。
	WikiUpdateShort string
	// WikiReadShort 是 read 命令说明。
	WikiReadShort string
	// WikiSearchShort 是 search 命令说明。
	WikiSearchShort string
	// WikiQueryShort 是 query 命令说明。
	WikiQueryShort string
	// WikiGlossaryShort 是 glossary 分组说明。
	WikiGlossaryShort string
	// WikiGlossaryKeywordsShort 是 glossary keywords 命令说明。
	WikiGlossaryKeywordsShort string
	// WikiRepoShort 是 repo 分组说明。
	WikiRepoShort string
	// WikiRepoInitShort 是 repo init 命令说明。
	WikiRepoInitShort string
	// WikiRepoAddShort 是 repo add 命令说明。
	WikiRepoAddShort string
	// WikiRepoLinkShort 是 repo link 命令说明。
	WikiRepoLinkShort string
	// WikiRepoUseShort 是 repo use 命令说明。
	WikiRepoUseShort string
	// WikiRepoInfoShort 是 repo info 命令说明。
	WikiRepoInfoShort string
	// WikiSkillShort 是 skill 分组说明。
	WikiSkillShort string
	// WikiSkillInstallShort 是 skill install 命令说明。
	WikiSkillInstallShort string
	// WikiCheckShort 是 check 命令说明。
	WikiCheckShort string
	// FlagAgent 是 agent 参数说明。
	FlagAgent string
	// FlagCodeDir 是 code-dir 参数说明。
	FlagCodeDir string
	// FlagYes 是 yes 参数说明。
	FlagYes string
	// FlagGlobal 是 global 参数说明。
	FlagGlobal string
	// FlagWikiType 是 skill 类型参数说明。
	FlagWikiType string
	// FlagWikiInitMode 是 init mode 参数说明。
	FlagWikiInitMode string
	// FlagWikiRepoSource 是 init 关联来源类型参数说明。
	FlagWikiRepoSource string
	// FlagPath 是本地路径参数说明。
	FlagPath string
	// PromptWikiProjectName 是 init 交互式项目名提示。
	PromptWikiProjectName string
	// PromptWikiInitMode 是 init 交互式文档库模式提示。
	PromptWikiInitMode string
	// PromptWikiType 是 init 安装 skill 前的 skill 类型提示。
	PromptWikiType string
	// PromptWikiAgent 是 init 交互式 agent 提示。
	PromptWikiAgent string
	// PromptWikiCodeDirs 是 init 交互式代码目录提示。
	PromptWikiCodeDirs string
	// PromptWikiRepoSource 是 init 关联文档库来源提示。
	PromptWikiRepoSource string
	// PromptWikiRepoLocalPath 是 init 本地文档库路径提示。
	PromptWikiRepoLocalPath string
	// PromptWikiRepoRemoteURL 是 init 远端文档库 URL 提示。
	PromptWikiRepoRemoteURL string
	// PromptWikiRepoLinkCode 是 init 是否关联代码库提示。
	PromptWikiRepoLinkCode string
	// PromptWikiRepoLinkMore 是 init 是否继续关联代码库提示。
	PromptWikiRepoLinkMore string
	// PromptWikiRepoCodeName 是 init 代码库标识提示。
	PromptWikiRepoCodeName string
	// PromptWikiRepoCodePath 是 init 代码库路径提示。
	PromptWikiRepoCodePath string
	// PromptWikiScope 是 init 交互式 skill 安装范围提示。
	PromptWikiScope string
	// PromptSelectWikiSkills 是 init 交互式 skill 多选提示。
	PromptSelectWikiSkills string
	// TitleWikiSummary 是 init 摘要块标题。
	TitleWikiSummary string
	// TitleQMDManualDownload 是 qmd 模型下载提示块标题。
	TitleQMDManualDownload string
	// ProjectLabel 是项目级 scope 和摘要字段展示名。
	ProjectLabel string
	// WikiTypeLabel 是 skill 类型摘要字段展示名。
	WikiTypeLabel string
	// SourceLabel 是文档库目录摘要字段展示名。
	SourceLabel string
	// AgentLabel 是 agent 摘要字段展示名。
	AgentLabel string
	// WikiCodeDirsLabel 是代码目录摘要字段展示名。
	WikiCodeDirsLabel string
	// ScopeLabel 是安装范围摘要字段展示名。
	ScopeLabel string
	// GlobalLabel 是全局级 scope 展示名。
	GlobalLabel string
	// InstallInProject 是项目级安装范围选项文案。
	InstallInProject string
	// InstallInHome 是全局级安装范围选项文案。
	InstallInHome string
	// WikiInitModeCreateLabel 是新建文档库模式选项文案。
	WikiInitModeCreateLabel string
	// WikiInitModeLinkLabel 是关联文档库模式选项文案。
	WikiInitModeLinkLabel string
	// WikiRepoSourceLocalLabel 是本地文档库来源选项文案。
	WikiRepoSourceLocalLabel string
	// WikiRepoSourceRemoteLabel 是远端文档库来源选项文案。
	WikiRepoSourceRemoteLabel string
	// WikiRepoLinkCodeLabel 是关联代码库选项文案。
	WikiRepoLinkCodeLabel string
	// WikiRepoContinueLabel 是跳过关联代码库选项文案。
	WikiRepoContinueLabel string
	// WikiRepoLinkAnotherLabel 是继续关联代码库选项文案。
	WikiRepoLinkAnotherLabel string
	// WikiRepoFinishLabel 是结束关联代码库选项文案。
	WikiRepoFinishLabel string
	// StepCreatingWikiProject 是创建 Wikimesh 文档库步骤文案。
	StepCreatingWikiProject string
	// StepInstallingWikiSkills 是安装 Wikimesh skills 步骤文案。
	StepInstallingWikiSkills string
	// StepFetchingWikiSkillsFmt 是获取 Wikimesh skills 来源提示模板。
	StepFetchingWikiSkillsFmt string
	// CreatedFmt 是创建完成步骤文案模板。
	CreatedFmt string
	// WikiInstalledSkillsFmt 是 Wikimesh skill 安装完成文案模板。
	WikiInstalledSkillsFmt string
	// QMDManualDownloadHint 是 qmd 模型手动下载提示。
	QMDManualDownloadHint string
	// QMDManualDownloadCommand 是 qmd 模型手动下载命令。
	QMDManualDownloadCommand string
	// Done 是命令完成提示。
	Done string
	// Cancelled 是交互取消后的展示文案。
	Cancelled string
	// NoMatchesFound 是搜索多选未匹配时的展示文案。
	NoMatchesFound string
	// SearchLabel 是搜索输入标签。
	SearchLabel string
	// SelectedLabel 是已选项标签。
	SelectedLabel string
	// SelectedNone 是未选择任何项时的展示文案。
	SelectedNone string
	// MoreSelectedFmt 是已选项过多时的汇总模板。
	MoreSelectedFmt string
	// AlwaysIncludedSuffix 是固定包含区域提示。
	AlwaysIncludedSuffix string
	// MultiSelectHelp 是多选交互帮助文案。
	MultiSelectHelp string
	// SingleSelectHelp 是单选交互帮助文案。
	SingleSelectHelp string
	// FlagRoot 是 root 参数说明。
	FlagRoot string
	// FlagProject 是 project 参数说明。
	FlagProject string
	// FlagView 是 view 参数说明。
	FlagView string
	// FlagFormat 是 format 参数说明。
	FlagFormat string
	// FlagRemote 是 remote 参数说明。
	FlagRemote string
	// FlagCollectionName 是 collection name 参数说明。
	FlagCollectionName string
	// FlagCollectionPath 是 collection path 参数说明。
	FlagCollectionPath string
	// FlagCollectionMask 是 collection mask 参数说明。
	FlagCollectionMask string
	// FlagCollectionInclude 是 collection include 参数说明。
	FlagCollectionInclude string
	// FlagLimit 是 limit 参数说明。
	FlagLimit string
	// FlagAll 是 all 参数说明。
	FlagAll string
	// FlagMinScore 是 min-score 参数说明。
	FlagMinScore string
	// FlagCollectionFilter 是 collection filter 参数说明。
	FlagCollectionFilter string
	// FlagRawVector 是 raw vector 参数说明。
	FlagRawVector string
	// FlagQueries 是 queries 参数说明。
	FlagQueries string
	// FlagSearchQuery 是 search-query 参数说明。
	FlagSearchQuery string
	// FlagIntent 是 intent 参数说明。
	FlagIntent string
	// FlagCandidateLimit 是 candidate-limit 参数说明。
	FlagCandidateLimit string
	// FlagExplain 是 explain 参数说明。
	FlagExplain string
	// FlagNoRerank 是 no-rerank 参数说明。
	FlagNoRerank string
	// FlagForce 是 force 参数说明。
	FlagForce string
	// FlagProvider 是 provider 参数说明。
	FlagProvider string
	// FlagModel 是 model 参数说明。
	FlagModel string
	// FlagCommand 是 command 参数说明。
	FlagCommand string
	// FlagDimensions 是 dimensions 参数说明。
	FlagDimensions string
	// FlagLibPath 是 lib 参数说明。
	FlagLibPath string
	// FlagProcessor 是 processor 参数说明。
	FlagProcessor string
	// FlagVersion 是 version 参数说明。
	FlagVersion string
	// FlagOS 是 os 参数说明。
	FlagOS string
	// FlagUpgrade 是 upgrade 参数说明。
	FlagUpgrade string
	// FlagPull 是 pull 参数说明。
	FlagPull string
	// HelpUsage 是 help 的用法标题。
	HelpUsage string
	// HelpAvailableCommands 是 help 的子命令标题。
	HelpAvailableCommands string
	// HelpFlags 是 help 的本地参数标题。
	HelpFlags string
	// HelpGlobalFlags 是 help 的全局参数标题。
	HelpGlobalFlags string
	// HelpMoreInfoFmt 是 help 的更多信息提示模板。
	HelpMoreInfoFmt string
	// OutputWikiCreatedFmt 是 init 创建文档库后的状态输出模板。
	OutputWikiCreatedFmt string
	// OutputWikiRepoRootFmt 是 repo init 的配置目录输出模板。
	OutputWikiRepoRootFmt string
	// OutputWikiRepoSavedFmt 是 repo add 的保存状态输出模板。
	OutputWikiRepoSavedFmt string
	// OutputWikiRepoLinkedFmt 是 repo link 的关联状态输出模板。
	OutputWikiRepoLinkedFmt string
	// OutputWikiRepoActiveFmt 是 repo use 的激活来源输出模板。
	OutputWikiRepoActiveFmt string
	// OutputWikiSkillsInstalledFmt 是 skill install/update 的安装输出模板。
	OutputWikiSkillsInstalledFmt string
	// OutputWikiCheckIssueFmt 是文档校验问题输出模板。
	OutputWikiCheckIssueFmt string
	// OutputWikiCheckPassedFmt 是文档校验通过输出模板。
	OutputWikiCheckPassedFmt string
	// OutputWikiUpdateDoneFmt 是二进制自更新完成输出模板。
	OutputWikiUpdateDoneFmt string
	// OutputWikiUpdateDeferredFmt 是 Windows 延迟替换提示模板。
	OutputWikiUpdateDeferredFmt string
	// OutputQMDCollectionFmt 是 qmd collection update 的集合标题输出模板。
	OutputQMDCollectionFmt string
	// OutputQMDUpdateStartFmt 是 qmd update 的集合数量输出模板。
	OutputQMDUpdateStartFmt string
	// OutputQMDUpdateDone 是 qmd update 完成提示。
	OutputQMDUpdateDone string
	// OutputQMDIndexedFmt 是 qmd collection update 的索引统计输出模板。
	OutputQMDIndexedFmt string
	// OutputQMDStatusTitle 是 qmd status 标题。
	OutputQMDStatusTitle string
	// OutputQMDStatusIndexFmt 是 qmd status 索引路径输出模板。
	OutputQMDStatusIndexFmt string
	// OutputQMDStatusDocumentsFmt 是 qmd status 文档统计输出模板。
	OutputQMDStatusDocumentsFmt string
	// OutputQMDStatusPendingFmt 是 qmd status 待向量化统计输出模板。
	OutputQMDStatusPendingFmt string
	// OutputQMDStatusCollections 是 qmd status 集合标题。
	OutputQMDStatusCollections string
	// OutputQMDStatusCollectionFmt 是 qmd status 单集合输出模板。
	OutputQMDStatusCollectionFmt string
	// OutputQMDStatusCollectionContextFmt 是 qmd status 集合上下文输出模板。
	OutputQMDStatusCollectionContextFmt string
	// OutputQMDEmbedHint 是索引变化后的 embedding 提示。
	OutputQMDEmbedHint string
	// OutputQMDNoCollections 是 embed 未发现集合时的提示。
	OutputQMDNoCollections string
	// OutputQMDModelFmt 是 embed 使用模型的输出模板。
	OutputQMDModelFmt string
	// OutputQMDEmbeddedFmt 是单集合 embedding 统计输出模板。
	OutputQMDEmbeddedFmt string
	// OutputQMDEmbeddedTotalFmt 是多集合 embedding 汇总输出模板。
	OutputQMDEmbeddedTotalFmt string
	// OutputQMDEmbedDone 是 embedding 完成提示。
	OutputQMDEmbedDone string
	// OutputQMDDownloadingFmt 是模型或库下载开始输出模板。
	OutputQMDDownloadingFmt string
	// OutputQMDDownloadedFmt 是模型下载完成输出模板。
	OutputQMDDownloadedFmt string
	// OutputQMDExistsFmt 是模型或库已存在输出模板。
	OutputQMDExistsFmt string
	// OutputQMDInstallingLibFmt 是 llama.cpp 运行时库安装输出模板。
	OutputQMDInstallingLibFmt string
	// OutputQMDInstalledFmt 是运行时库安装完成输出模板。
	OutputQMDInstalledFmt string
	// ErrorUnsupportedInitModeFmt 是不支持 init mode 的错误模板。
	ErrorUnsupportedInitModeFmt string
	// ErrorProjectRequired 是项目名必填错误。
	ErrorProjectRequired string
	// ErrorUnsupportedAgentFmt 是不支持 agent 的错误模板。
	ErrorUnsupportedAgentFmt string
	// ErrorUnsupportedWikiSourceFmt 是不支持文档库来源类型的错误模板。
	ErrorUnsupportedWikiSourceFmt string
	// ErrorLocalWikiSourcePathRequired 是本地文档库路径必填错误。
	ErrorLocalWikiSourcePathRequired string
	// ErrorLocalWikiSourcePathNotDirFmt 是本地文档库路径非目录错误模板。
	ErrorLocalWikiSourcePathNotDirFmt string
	// ErrorRemoteWikiSourceURLRequired 是远端文档库 URL 必填错误。
	ErrorRemoteWikiSourceURLRequired string
	// ErrorCodeRepoSlugRequired 是代码库标识必填错误。
	ErrorCodeRepoSlugRequired string
	// ErrorCodeRepoPathRequired 是代码库路径必填错误。
	ErrorCodeRepoPathRequired string
	// ErrorCodeDirNotDirFmt 是代码库路径非目录错误模板。
	ErrorCodeDirNotDirFmt string
}

var defaultCatalog = Catalog{
	RootShort:                           "Wikimesh 知识库命令行",
	QMDShort:                            "管理 qmd 文档集合、索引和检索",
	CollectionShort:                     "管理已索引的文档集合",
	CollectionAddShort:                  "添加文档集合",
	CollectionListShort:                 "列出文档集合",
	CollectionRemoveShort:               "移除文档集合",
	CollectionUpdateShort:               "刷新文档集合索引",
	QMDStatusShort:                      "查看 qmd 索引和集合状态",
	QMDUpdateShort:                      "刷新所有 qmd 集合索引",
	SearchShort:                         "搜索默认文档集合",
	VSearchShort:                        "向量搜索默认文档集合",
	QueryShort:                          "对默认文档集合执行混合查询",
	EmbedShort:                          "为已索引文档生成向量",
	ModelShort:                          "管理本地 GGUF 模型",
	ModelDownloadShort:                  "下载配置中的模型到 .wikimesh/models",
	ModelLibShort:                       "管理 llama.cpp 运行时库",
	ModelLibInstallShort:                "安装 yzma llama.cpp 运行时库到 .wikimesh/lib",
	WikiInitShort:                       "初始化 Wikimesh 工作区",
	WikiUpdateShort:                     "更新当前 Wikimesh 可执行文件",
	WikiReadShort:                       "读取 Wikimesh 页面视图",
	WikiSearchShort:                     "搜索 Wikimesh 知识库",
	WikiQueryShort:                      "对 Wikimesh 项目执行语义查询",
	WikiGlossaryShort:                   "查看 Wikimesh 术语表",
	WikiGlossaryKeywordsShort:           "列出 Wikimesh 术语关键词",
	WikiRepoShort:                       "管理 Wikimesh 项目来源",
	WikiRepoInitShort:                   "初始化 Wikimesh 项目来源配置目录",
	WikiRepoAddShort:                    "添加 Wikimesh 项目来源",
	WikiRepoLinkShort:                   "关联代码仓到 Wikimesh 项目",
	WikiRepoUseShort:                    "选择 Wikimesh 项目来源",
	WikiRepoInfoShort:                   "查看 Wikimesh 项目来源信息",
	WikiSkillShort:                      "管理 Wikimesh skills",
	WikiSkillInstallShort:               "安装 Wikimesh skills",
	WikiCheckShort:                      "校验 Wikimesh 文档",
	FlagAgent:                           "目标 Agent：codex、cursor、claude",
	FlagCodeDir:                         "代码仓目录，可重复传入多个目录",
	FlagYes:                             "跳过交互确认",
	FlagGlobal:                          "安装到主目录而不是当前项目",
	FlagWikiType:                        "Skill 类型，例如 devwiki",
	FlagWikiInitMode:                    "初始化方式：create 新建文档库，link 关联文档库",
	FlagWikiRepoSource:                  "关联文档库来源：local、remote",
	FlagPath:                            "本地文档库路径",
	PromptWikiProjectName:               "Wikimesh 项目名称",
	PromptWikiInitMode:                  "选择文档库初始化方式",
	PromptWikiType:                      "选择 Skill 类型",
	PromptWikiAgent:                     "选择 Wikimesh Agent（可多选）",
	PromptWikiCodeDirs:                  "代码目录（逗号分隔，可留空）",
	PromptWikiRepoSource:                "选择文档库来源",
	PromptWikiRepoLocalPath:             "本地文档库路径",
	PromptWikiRepoRemoteURL:             "远端文档库 URL",
	PromptWikiRepoLinkCode:              "关联代码库？",
	PromptWikiRepoLinkMore:              "继续关联代码库？",
	PromptWikiRepoCodeName:              "代码库标识",
	PromptWikiRepoCodePath:              "代码库路径",
	PromptWikiScope:                     "选择 Wikimesh skill 安装范围",
	PromptSelectWikiSkills:              "选择要安装的 Wikimesh skills",
	TitleWikiSummary:                    "Wikimesh 初始化摘要",
	TitleQMDManualDownload:              "QMD 模型下载",
	ProjectLabel:                        "项目级",
	WikiTypeLabel:                       "Skill 类型",
	SourceLabel:                         "文档库目录",
	AgentLabel:                          "Agent",
	WikiCodeDirsLabel:                   "代码目录",
	ScopeLabel:                          "范围",
	GlobalLabel:                         "全局级",
	InstallInProject:                    "安装到当前项目",
	InstallInHome:                       "安装到主目录",
	WikiInitModeCreateLabel:             "新建文档库",
	WikiInitModeLinkLabel:               "关联文档库",
	WikiRepoSourceLocalLabel:            "本地文档库",
	WikiRepoSourceRemoteLabel:           "远端文档库",
	WikiRepoLinkCodeLabel:               "关联代码库",
	WikiRepoContinueLabel:               "暂不关联",
	WikiRepoLinkAnotherLabel:            "继续关联",
	WikiRepoFinishLabel:                 "完成",
	StepCreatingWikiProject:             "创建 Wikimesh 文档库",
	StepFetchingWikiSkillsFmt:           "获取 Wikimesh skills：%s",
	StepInstallingWikiSkills:            "安装 Wikimesh skills",
	CreatedFmt:                          "已创建 Wikimesh 文档库：%s",
	WikiInstalledSkillsFmt:              "已安装 %s 的 %d 个 Wikimesh skills",
	QMDManualDownloadHint:               "如需手动下载 QMD models，请进入新建的 Wikimesh 工作区后执行：",
	QMDManualDownloadCommand:            "wikimesh qmd model download all",
	Done:                                "完成！",
	Cancelled:                           "已取消",
	NoMatchesFound:                      "没有匹配项",
	SearchLabel:                         "搜索",
	SelectedLabel:                       "已选择",
	SelectedNone:                        "无",
	MoreSelectedFmt:                     "%s 等 %d 项",
	AlwaysIncludedSuffix:                "始终包含",
	MultiSelectHelp:                     "输入筛选，空格选择，回车确认，Esc 取消",
	SingleSelectHelp:                    "上下选择，回车确认，Esc 取消",
	FlagRoot:                            "Wikimesh 工作区根目录",
	FlagProject:                         "Wikimesh 项目名称或 slug；设置后按用户级项目配置解析本地工作区",
	FlagView:                            "页面视图：card、core、explain",
	FlagFormat:                          "输出格式",
	FlagRemote:                          "远端 Wikimesh API 地址",
	FlagCollectionName:                  "文档集合名称",
	FlagCollectionPath:                  "文档集合根目录",
	FlagCollectionMask:                  "glob 匹配规则",
	FlagCollectionInclude:               "逗号分隔的 include glob 列表",
	FlagLimit:                           "最大返回数量",
	FlagAll:                             "返回全部搜索结果",
	FlagMinScore:                        "最低分数阈值",
	FlagCollectionFilter:                "文档集合过滤，可重复传入多个集合",
	FlagRawVector:                       "使用原始向量搜索，不执行查询扩展",
	FlagQueries:                         "预扩展查询 JSON",
	FlagSearchQuery:                     "辅助关键词 query，可重复传入多个 query 后只做 FTS 召回并参与 RRF 融合",
	FlagIntent:                          "领域意图提示",
	FlagCandidateLimit:                  "重排前最大候选数量",
	FlagExplain:                         "输出 RRF/重排解释信息",
	FlagNoRerank:                        "跳过 reranker，返回 RRF 位置分",
	FlagForce:                           "重建已有向量",
	FlagProvider:                        "覆盖 embedding provider",
	FlagModel:                           "覆盖 embedding 模型",
	FlagCommand:                         "覆盖 embedding 命令",
	FlagDimensions:                      "覆盖 embedding 维度",
	FlagLibPath:                         "llama.cpp 运行时库目录",
	FlagProcessor:                       "处理器类型：auto、cpu、metal、cuda、vulkan、rocm",
	FlagVersion:                         "llama.cpp release 版本",
	FlagOS:                              "运行时系统：linux、darwin、windows、bookworm、trixie",
	FlagUpgrade:                         "即使运行时库已存在也重新下载",
	FlagPull:                            "索引前执行 collection 的 update 命令",
	HelpUsage:                           "用法",
	HelpAvailableCommands:               "可用命令",
	HelpFlags:                           "参数",
	HelpGlobalFlags:                     "全局参数",
	HelpMoreInfoFmt:                     "使用 \"%s --help\" 查看更多信息。",
	OutputWikiCreatedFmt:                "已创建 Wikimesh 工作区：%s\n",
	OutputWikiRepoRootFmt:               "Wikimesh 项目来源配置目录：%s\n",
	OutputWikiRepoSavedFmt:              "已保存 Wikimesh 项目：%s\n",
	OutputWikiRepoLinkedFmt:             "已关联代码仓：%s\n",
	OutputWikiRepoActiveFmt:             "当前来源：%s\n",
	OutputWikiSkillsInstalledFmt:        "已为 %s 安装 %s 的 %d 个 Wikimesh skill\n",
	OutputWikiCheckIssueFmt:             "错误 %s\n",
	OutputWikiCheckPassedFmt:            "Wikimesh 文档校验通过（%d 个文件）\n",
	OutputWikiUpdateDoneFmt:             "%s完成%s 已更新 %s -> %s\n",
	OutputWikiUpdateDeferredFmt:         "%s完成%s 已下载 %s，退出当前进程后会替换 %s；请重新打开终端。\n",
	OutputQMDCollectionFmt:              "集合：%s\n",
	OutputQMDUpdateStartFmt:             "更新 %d 个集合...\n\n",
	OutputQMDUpdateDone:                 "全部集合已更新。\n",
	OutputQMDIndexedFmt:                 "索引：%d 个新增/更新，%d 个未变化，%d 个已移除\n",
	OutputQMDStatusTitle:                "QMD 状态",
	OutputQMDStatusIndexFmt:             "索引：%s\n",
	OutputQMDStatusDocumentsFmt:         "文档：%d 个文件已索引，%d 个向量\n",
	OutputQMDStatusPendingFmt:           "待向量化：%d 个文档\n",
	OutputQMDStatusCollections:          "集合：",
	OutputQMDStatusCollectionFmt:        "  %s：%d 个文件，pattern=%s，更新=%s\n",
	OutputQMDStatusCollectionContextFmt: "    上下文：%d\n",
	OutputQMDEmbedHint:                  "\n运行 'wikimesh qmd embed' 更新向量\n",
	OutputQMDNoCollections:              "未发现集合。请先运行 'wikimesh qmd collection add .' 索引 Markdown 文件。",
	OutputQMDModelFmt:                   "模型：%s\n\n",
	OutputQMDEmbeddedFmt:                "向量：%d 个 chunk，%d 个文档已检查，%d 个未变化\n\n",
	OutputQMDEmbeddedTotalFmt:           "向量：%d 个 chunk 总计，%d 个文档已检查，%d 个未变化\n",
	OutputQMDEmbedDone:                  "全部向量已更新。\n",
	OutputQMDDownloadingFmt:             "正在下载：%s -> %s\n",
	OutputQMDDownloadedFmt:              "已下载：%s -> %s\n",
	OutputQMDExistsFmt:                  "已存在：%s\n",
	OutputQMDInstallingLibFmt:           "正在安装 llama.cpp：processor=%s version=%s -> %s\n",
	OutputQMDInstalledFmt:               "已安装：%s\n",
	ErrorUnsupportedInitModeFmt:         "不支持的初始化方式：%q",
	ErrorProjectRequired:                "项目名称不能为空",
	ErrorUnsupportedAgentFmt:            "不支持的 Agent：%q",
	ErrorUnsupportedWikiSourceFmt:       "不支持的文档库来源：%q",
	ErrorLocalWikiSourcePathRequired:    "本地文档库路径不能为空",
	ErrorLocalWikiSourcePathNotDirFmt:   "本地文档库路径不是目录：%q",
	ErrorRemoteWikiSourceURLRequired:    "远端文档库 URL 不能为空",
	ErrorCodeRepoSlugRequired:           "代码库标识不能为空",
	ErrorCodeRepoPathRequired:           "代码库路径不能为空",
	ErrorCodeDirNotDirFmt:               "代码库路径不是目录：%q",
}

// Messages 返回当前 CLI 文案目录；后续多语言支持只需要切换这里的来源。
func Messages() Catalog {
	return defaultCatalog
}

// ScopeText 返回当前语言下的安装范围展示文案。
func ScopeText(global bool) string {
	if global {
		return Messages().GlobalLabel
	}
	return Messages().ProjectLabel
}
