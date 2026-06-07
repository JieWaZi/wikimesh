// Package qmd 提供 qmd 风格的文档集合检索 SDK。
//
// SDK 入口是 NewStore 创建的 Store。调用方可以通过 Store 管理 collection、
// 更新文档索引、生成向量、执行关键词检索、向量检索以及混合 Query。
// 底层 SQLite、FTS、chunk、向量表和本地模型运行时均封装在包内或子包内，
// 外部调用方只需要依赖 `github.com/JieWaZi/wikimesh/pkg/qmd` 的公开类型和接口。
package qmd
