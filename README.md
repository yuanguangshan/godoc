# tools 模块

## 模块职责概述

`tools/` 是 **Tmux-FSM 的开发工具集合**，负责提供辅助开发、文档生成、测试等工具。该模块包含了项目维护和开发过程中使用的各种实用工具，旨在提高开发效率和项目可维护性。

---

🚀 开始安装 gen-docs...
✓ Go 版本: go version go1.25.5 darwin/amd64
📦 编译 gen-docs...
📍 安装目录: /usr/local/bin
📥 安装 gen-docs
🔗 创建 gd 快捷命令

✅ 安装完成！

现在你可以在任意目录使用：
  gen-docs     # 完整命令
  gd           # 快捷命令

示例：
  gd
  gd -i md,go                  # 只包含特定后缀
  gd -x exe,bin                # 排除特定后缀
  gd -m _test.go               # 模糊匹配：提取所有测试文件
  gd -m _test.go -xm vendor/   # 复合匹配：包含测试但排除第三方库
  gd -ns                       # 不扫描子目录

---

主要职责包括：
- 提供项目文档生成工具
- 包含开发辅助脚本
- 提供测试和验证工具
- 维护项目构建和部署工具

## 核心设计思想

- **开发辅助**: 为开发过程提供便利工具
- **文档生成**: 自动化生成项目文档
- **可维护性**: 工具本身易于维护和扩展
- **实用性**: 解决实际开发中的痛点问题

## 文件结构说明

### `gen-docs.go`
- 项目文档生成工具
- 主要功能：
  - 扫描项目目录结构
  - 生成项目文档快照
  - 支持多种文件格式过滤
  - 提供二进制文件检测
- 用途：
  - 生成项目整体文档
  - 为架构审计提供材料
  - 创建代码快照用于审查

### `install-gen-docs.sh`
- gen-docs 工具安装脚本
- 主要功能：
  - 检查 Go 环境
  - 编译 gen-docs 工具
  - 安装到系统路径
  - 创建 `gd` 快捷命令
- 用途：
  - 简化工具安装过程
  - 提供便捷的使用方式

## 工具特性

### 文档生成
- 支持项目级文档快照生成
- 自动过滤二进制文件
- 支持文件大小限制
- 提供详细的统计信息

### 易用性
- 提供简洁的命令行接口
- 支持多种过滤选项
- 自动输出文件命名
- 提供进度指示

## 在整体架构中的角色

Tools 模块是项目的辅助开发层，它为开发者提供了便利的工具集。Tools 提供了：
- 项目文档的自动化生成
- 开发流程的简化
- 代码审查和审计的支持
- 项目可维护性的提升


改进对照表
#	问题	改进方式
1	shouldIgnoreFile 死代码	已删除，所有过滤逻辑统一在 scanDirectory 内联完成
2	每个文件被读取两次	采用临时文件方案：写入阶段用 os.ReadFile 一次性读取内容，同时统计行数并写入 temp file，最终组装输出时从 temp file 复制，源文件只读一次
3	.min.* 被错误当成二进制	从 isBinaryFile 中移除了 .min. 硬编码检查
4	代码块固定四反引号	determineFence() 动态扫描文件内容中反引号最大连续长度，fence 长度 = max+1（最小为 3）
5	锚点生成冲突	generateAnchor() 用 - 替代 / 和 .（而非直接删除），并去重连续 -，避免 src/main.go 与 src/maingo 冲突
6	缺少 .gitignore 集成	新增 loadGitignorePatterns() 解析项目根目录的 .gitignore，支持基本模式（名称匹配、路径匹配、通配符、目录专属 /），同时在目录和文件两个层级应用
7	缺少 dry-run 模式	新增 --dry-run 标志，只输出文件列表和预估统计，不实际写入
8	错误处理不严谨	filepath.Abs、filepath.Rel 的错误不再用 _ 忽略，改为在 verbose 模式下记录；写入阶段失败的文件从统计和 TOC 中正确剔除
9	安装脚本缺少卸载/升级	新增 --uninstall 参数支持卸载；安装前检查 gen-docs.go 是否存在；检测并提示已有版本；安装后验证并显示版本号
10	忽略模式不可配置	新增 --no-default-ignore 禁用内置忽略规则；新增 --ignore 追加自定义忽略模式；新增 --no-gitignore 禁用 gitignore 加载
11	语言映射不够全	扩充了 languageMap（vue、svelte、dart、lua、perl、elixir、erlang、haskell、ocaml、clojure、protobuf、graphql、hcl 等）；detectLanguage 增加了对 Dockerfile、Makefile 等无扩展名文件的识别