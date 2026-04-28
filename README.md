# Sourcepack (gdoc)

**一键将项目代码库转换为 AI 可读的 Markdown 快照。**

Sourcepack 是一个极简、高性能的工具，用于扫描项目目录并将代码合并为单个 Markdown 文件，方便你快速喂给 LLM (GPT/Claude) 或进行代码审计。

## 安装

```bash
npm install -g sourcepack
# 或
bash install.sh
```

## 使用

```bash
sourcepack          # 主命令（与包名一致）
gdoc                # 快捷命令
gdoc -i go,md       # 只包含 Go 和 Markdown 文件
gdoc -x exe,bin     # 排除特定后缀
gdoc -X vendor      # 排除指定目录关键字
gdoc -n             # 不扫描子目录
gdoc --dry-run      # 预览文件列表
gdoc -s             # 显示详细统计（文件/目录/语言/Token 分布），不生成文件
gdoc -c             # 直接复制到剪贴板，不写文件
gdoc -p             # 推送到远端中继（需设置 SOURCEPACK_PUSH_URL 和 SOURCEPACK_AUTH_KEY）
gdoc -s -p          # 只推送统计数据到远端
gdoc -v             # 详细模式（显示扫描过程）
```

> `sourcepack` 和 `gdoc` 两个命令完全等效。

## 生成的快照包含

- **项目结构树** — 目录优先、字母排序的树形图，快速把握项目全貌
- **文件目录 (TOC)** — 带锚点跳转的文件索引
- **完整源码** — 自动语法高亮、智能处理嵌套代码块
- **项目统计** — `-s` 输出多维度统计（文件、目录、语言、Token 分布）

## 特性

- **极速**: Go 编写，秒级处理万行代码。
- **智能**: 自动处理 `.gitignore` 和二进制文件。
- **统计**: `-s` 多维度代码统计（Token 预估、目录占比、语言分布）。
- **便捷**: `-c` 一键复制到剪贴板，`-p` 一键推送到远端。
- **清晰**: 自动生成带跳转链接的项目目录树。

---
*Simple, Fast, and AI-Friendly.*
