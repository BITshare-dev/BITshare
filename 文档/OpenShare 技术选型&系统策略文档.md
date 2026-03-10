# OpenShare 技术选型 / 系统策略文档

## 一、核心技术栈

### 前端

- Vue 3
- Vite
- TypeScript
- Tailwind CSS

### 后端

- Go
- Gin
- database/sql

### 数据库

- SQLite

### 文件存储

- Linux Local File System

---

## 二、使用场景

OpenShare 是一个轻量级、自托管的资料共享系统，主要用于校园网、实验室或组织内部网络环境，实现学习资料的共建与共享。

---

## 三、角色设置

系统包含三类角色：

**Guest（普通用户）**

**Admin（管理员）**

**Super Admin（超级管理员）**

认证方式：

- JWT Token
- bcrypt 密码加密
- 请求 Header：Authorization: Bearer Token

---

## 四、完整功能流程

### 浏览资料

用户访问系统后可以浏览资料列表。

资料展示信息：

- 标题
- Tag
- 上传时间
- 下载量
- 文件大小

系统不展示上传者信息。

### 下载资料

支持：

- 单文件下载 HTTP Stream（http.ServeFile）
- 打包下载/批量下载 Streaming ZIP（archive/zip + io.Copy）

下载行为会记录下载次数。

### 搜索资料

**搜索策略**：

关键词搜索

模糊匹配

Tag 限定搜索、支持 Tag 组合搜索、文件继承父文件夹 Tag 参与搜索

搜索对象包含文件和文件夹&#x20;

支持限定目录范围搜索&#x20;

搜索忽略大小写&#x20;

结果按相关度 + 下载量排序

**搜索技术：**

- SQLite FTS5（Full Text Search）（文件名、文件夹名、tag等）
- 前缀匹配（Prefix Search）
- LIKE 模糊匹配（辅助搜索）

### 上传资料

上传技术：

- multipart/form-data
- Gin FormFile
- SaveUploadedFile

上传流程：

上传 → staging → 创建 submission → 管理员审核 → rename 到 repository

### 查询上传记录

用户可通过回执码查询上传记录。

展示信息：

- 文件标题
- 审核状态
- 上传时间
- 历史上传文件的下载量

审核状态：

- pending
- approved
- rejected

浏览器会缓存回执码用于再次查询。

### 管理员审核

管理员审核上传资料。

审核结果：

- approved
- rejected

仅通过审核的资料会公开展示。

### 举报资料

用户可举报：

- 文件
- 文件夹

流程：

用户提交举报 → 状态 pending → 管理员审核

处理结果：

- 举报成立 → 资源下架
- 举报驳回 → 保持资源

---

## 五、文件管理模式

系统采用 **本地文件存储 + 数据库存储元数据** 的模式。

### 存储结构

```
/data/openshare

repository   文件仓库
staging      上传暂存
trash        删除回收
```

### 文件命名策略

- 磁盘文件名：保持原始文件名
- 数据库文件 ID：UUID 同时也包含原始文件名

规则：

- 同目录文件名冲突 → 返回 409 Conflict（需重命名）

### 元数据管理

文件元数据统一存储于 SQLite。

---

## 六、Tag 系统

Tag 为独立实体，由数据库统一管理。

规则：

- 文件和文件夹均可绑定 Tag
- 文件继承父文件夹 Tag
- Tag 不允许重名（忽略大小写）
- 管理员可创建 Tag
- 用户可提交 Tag（需管理员审核）

---

## 七、公告系统

管理员可发布系统公告。

功能：

- 首页展示公告
- 支持编辑
- 支持删除
- 支持隐藏

---

## 八、系统安全策略

### 操作记录

系统记录：

- 上传者 IP
- 管理员操作 IP

用于安全审计。

### 防刷机制

以下接口进行访问频率限制：

- 上传
- 搜索

