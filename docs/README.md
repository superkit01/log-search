# 文档导航

| 分类 | 文档 | 适用场景 |
|---|---|---|
| 使用指南 | [用户手册](guides/用户手册.md) | 日志查询、分页、图表、分享与设置 |
| API 契约 | [查询接口](api/查询接口.md) | 请求参数、limit、游标、响应与错误 |
| 架构交接 | [开发交接文档](architecture/开发交接文档.md) | 存储输入字段、ES/Loki 映射及接入要求 |
| 外部集成 | [MCP 接入](integrations/MCP接入.md) | Agent 工具、多用户 API Key 与协议接入 |
| 集成设计 | [服务目录设计](integrations/服务目录设计.md) | namespace/env/app 到 GitLab 项目、分支和部署版本的事实源设计 |

项目启动、目录职责和开发命令见[项目 README](../README.md)。环境地址与凭据放在 Git 忽略的 `config.yaml` 中，通用配置参考根目录的 `config.example.yaml`。
