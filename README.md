# 项目管理 · 深信服实施交付工作台（Linux）

一个面向**个人实施交付**的本地项目管理工具，用来管住维保、加点、新实施、测试四类项目的全生命周期：
项目进度、风险问题、附件版本、客户网络信息、设备档案，外加验收 / 维保到期 / 长期未更新三类提醒。

BS 架构，Go 单二进制 + 内嵌前端，一条命令启动，数据全部留在本机。

![Go](https://img.shields.io/badge/Go-1.26+-00ADD8?logo=go&logoColor=white)
![SQLite](https://img.shields.io/badge/SQLite-pure--Go-003B57?logo=sqlite&logoColor=white)
![License](https://img.shields.io/badge/License-MIT-green)
![Platform](https://img.shields.io/badge/Linux-x64%20%7C%20arm64-555)

---

## 目录

- [分支说明](#分支说明)
- [功能特性](#功能特性)
- [技术栈](#技术栈)
- [快速开始（从 git clone 到跑起来）](#快速开始从-git-clone-到跑起来)
  - [第 0 步：准备环境](#第-0-步准备环境)
  - [第 1 步：克隆代码](#第-1-步克隆代码)
  - [第 2 步：编译](#第-2-步编译)
  - [第 3 步：启动](#第-3-步启动)
  - [第 4 步：首次配置](#第-4-步首次配置)
  - [第 5 步：验证与停止](#第-5-步验证与停止)
- [服务器部署（systemd）](#服务器部署systemd)
- [局域网访问](#局域网访问)
- [升级、迁移与卸载](#升级迁移与卸载)
- [数据与备份](#数据与备份)
- [目录结构](#目录结构)
- [数据模型](#数据模型)
- [安全说明](#安全说明)
- [已知限制与 Roadmap](#已知限制与-roadmap)
- [常见问题](#常见问题)
- [许可](#许可)

---

## 分支说明

| 分支 | 内容 |
|---|---|
| `For_Linux` | **Linux 版本（本分支）**：Linux 构建脚本、中文字体修复、systemd 部署与文档 |
| `main` | Windows 版本 |

两个分支的数据库结构与数据目录格式一致，停服务后整体拷贝数据目录即可互迁。
本 README 只写 Linux 用法，Windows 用法见 `main` 分支。

---

## 功能特性

### 项目
- 四类项目：**新实施 / 加点 / 测试 / 维保**，各自独立的阶段模板
- 状态、优先级、产品线（安全产品 / 云产品等）+ 具体产品名称，一个项目可挂多个产品
- 项目编号手填，不做合同金额 / 销售 / 售前等商务字段
- 项目关联：加点关联原项目、测试转实施、验收转维保
- 转化时自动继承：客户、产品、设备快照、网络信息版本、附件（物理复制为新版本）、进度记录（可选）、验收信息

### 客户档案
- 客户 → 多个联系人（姓名必填，电话 / 邮箱 / 角色 / 备注选填）
- **客户网络信息版本管理**：按客户整体出版本，自动生成字段级变更历史（修改前值 / 修改后值 / 操作类型 / 时间）
- **客户设备档案**：型号、序列号、账号密码、拓扑图（多张，图片或文件）、网络信息，改动留痕
- 项目引用网络信息的**指定版本**，不跟随最新版漂移

### 进度与问题
- 进度记录：日期、多阶段、当前进展、存在问题、下一步计划、信息同步（可空）、附件
- 首页取「最新一条进度记录的下一步计划」，为空显示「未定」
- 风险问题：等级 / 负责人 / 状态 / 关闭时间，关闭后仍保留在项目详情中

### 提醒与看板
- 提醒规则：维保到期前 **60 天**、长期未更新 **7 天**、验收前 **3 天**、长时间未结束 **14 天**
- 首页五区：长时间未结束项目、每个项目的下一步计划、本周进度记录、即将到期维保、未关闭问题
- 提醒中心支持标记已读 / 忽略，扫描幂等（同一条件不重复刷屏）

### 附件
- 类型：实施方案 / 拓扑图 / 配置备份 / 授权截图 / 验收单 / 现场照片 / 维保记录 / 其他
- **版本管理**：改一次留一版，旧版本可追溯；单个文件上限 300MB
- 图片与 PDF 在线预览

### 导出
- **Excel**（单工作簿 6 张表）：项目清单、进度记录、客户网络信息、设备清单、问题清单、维保到期清单
- **PDF**（4 类模板）：项目周报、验收报告、客户网络信息表、维保报告
- 验收报告 PDF 直接输出你上传的验收单原件，不额外生成汇总页
- 支持按筛选条件导出、勾选批量导出、批量改状态

### 其他
- 全局搜索：项目 / 客户 / 进度 / 问题 / 附件 / 设备
- 本地自动备份：频率、保留份数、是否含附件均可配
- 深色 / 浅色主题

---

## 技术栈

| 层 | 选型 | 说明 |
|---|---|---|
| 后端 | Go 1.26 + `net/http` 标准库路由 | 无 web 框架，路由用 `ServeMux` 的 method + 通配符模式 |
| 存储 | SQLite（`modernc.org/sqlite`） | **纯 Go 实现，`CGO_ENABLED=0`**，不需要 C 编译器，可交叉编译 amd64 / arm64 |
| 前端 | 原生 JS + 手写 CSS | 无构建步骤、无 npm 依赖，`embed` 打进二进制 |
| Excel | `github.com/xuri/excelize/v2` | 多 sheet、表头样式、冻结窗格、自动筛选 |
| PDF | `github.com/signintech/gopdf` | 运行时探测系统中文字体（会把 `.ttc` 拆成单字体、校验 cmap 覆盖中文），输出 Type0 / Identity-H + 内嵌 FontFile2 |
| 认证 | 标准库 PBKDF2-HMAC-SHA256 | 12 万次迭代，内存会话 Cookie |

代码量：Go ≈ 5.3k 行，前端 ≈ 2.6k 行，共 25 张表，37 个界面。

---

## 快速开始（从 git clone 到跑起来）

> 已经拿到现成的 `sangfor-pm` 单文件？可以跳过第 0～2 步，直接看 [第 3 步：启动](#第-3-步启动)。
> 仓库不提交编译产物，正常流程是 clone 后自己编译，大约 3～5 分钟。

### 第 0 步：准备环境

只需要两样东西：**Git** 和 **Go 1.26+**。不需要 Node、不需要 C 编译器、不需要单独安装数据库。

```bash
# Debian / Ubuntu
sudo apt update && sudo apt install -y git

# CentOS / RHEL / Fedora
sudo dnf install -y git
```

确认架构（决定下载哪个 Go 包）：

```bash
uname -m
# x86_64  -> amd64
# aarch64 -> arm64
```

安装 Go（**不要用发行版仓库的 `golang` 包**，版本通常低于 1.26）：

```bash
# 以 x86_64 为例；arm64 把包名里的 amd64 换成 arm64
wget https://go.dev/dl/go1.26.5.linux-amd64.tar.gz   # 版本以 https://go.dev/dl/ 的最新 1.26.x 为准
sudo rm -rf /usr/local/go
sudo tar -C /usr/local -xzf go1.26.5.linux-amd64.tar.gz
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
source ~/.bashrc
```

验证：

```bash
git --version
go version        # 需要 >= go1.26，例如 go version go1.26.5 linux/amd64
```

> 国内网络拉取 Go 模块可能较慢，可先设置代理（可选）：
> `go env -w GOPROXY=https://goproxy.cn,direct`

### 第 1 步：克隆代码

**必须指定 `For_Linux` 分支**，默认的 `main` 是 Windows 版：

```bash
git clone -b For_Linux https://github.com/yimu56/Sangfor_Project_Manager.git
cd Sangfor_Project_Manager
git branch --show-current      # 应输出 For_Linux
```

只想拿最新一份、不关心历史，可以加 `--depth 1` 加速：

```bash
git clone -b For_Linux --depth 1 https://github.com/yimu56/Sangfor_Project_Manager.git
```

已经 clone 过、当前在 `main` 的，切分支即可：

```bash
git fetch origin
git checkout For_Linux
```

> 克隆很慢或卡在 `TLS handshake`：检查网络 / 代理，或直接在 GitHub 页面切到 `For_Linux` 分支后
> **Code → Download ZIP**，解压进入目录，后续步骤完全一样。

目录里应该能看到 `main.go`、`go.mod`、`build.sh`、`start.sh`、`deploy/`、`web/` 等文件。

### 第 2 步：编译

编译会生成**一个二进制**，前端已经打包在里面，之后运行不需要 Go 和源文件。

```bash
bash build.sh
```

产物：当前目录下的 `sangfor-pm`。

等价手动命令：

```bash
CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o sangfor-pm .
```

交叉编译到 ARM 服务器（在 x86 机器上也可执行）：

```bash
GOOS=linux GOARCH=arm64 OUT=dist/sangfor-pm-linux-arm64 bash build.sh
```

编译成功会看到类似输出：

```
==> 使用 go1.26.5
==> 编译 linux/amd64 -> sangfor-pm
==> 完成：/home/you/Sangfor_Project_Manager/sangfor-pm（16613538 字节）
```

> 常见编译问题：
> - `go: command not found`：Go 没装好或 PATH 没生效，重开终端 / 检查 `echo $PATH`。
> - `Permission denied`：用 `bash build.sh`，或 `chmod +x build.sh`。
> - `$'\r': command not found`：脚本被转成了 CRLF，执行 `sed -i 's/\r$//' build.sh`。

### 第 3 步：启动

```bash
./sangfor-pm
```

如果提示 `Permission denied`：`chmod +x sangfor-pm`。

服务器 / 无桌面环境（不自动拉浏览器，允许局域网访问）：

```bash
./sangfor-pm -no-open -host 0.0.0.0 -port 8787
```

也可以直接用启动脚本（没编译会先编译，无桌面会自动加 `-no-open`）：

```bash
bash start.sh
```

启动后控制台会打印实际地址和数据目录，例如：

```
==================================================
  项目管理 · 深信服实施交付个人工作台  v1.0.0
  平台: linux/amd64
==================================================
数据目录: /root/.sangfor-pm
数据库  : /root/.sangfor-pm/data.db
临时目录: /root/.sangfor-pm/tmp
本机访问: http://127.0.0.1:8787
局域网访问（其他主机可用）:
    http://192.168.1.20:8787
--------------------------------------------------
```

- 浏览器一般会自动打开 `http://127.0.0.1:8787`；没打开就手动访问。
- 默认端口是 `8787`，被占用时会自动顺延（8788、8789…），**以控制台打印的地址为准**。
- 数据目录默认在 `~/.sangfor-pm`，第一次启动时自动创建。
- 停止服务：按 `Ctrl+C`，会看到「正在停止服务… / 服务已停止。」。

> 想把数据放到指定目录（服务器推荐）：
> `./sangfor-pm -data /var/lib/sangfor-pm`
> 详见 [数据与备份](#数据与备份) 和 [docs/linux-deploy.md](docs/linux-deploy.md)。

### 第 4 步：首次配置

浏览器打开后，按下面顺序点一遍就能开始用了（全程不需要改配置文件）：

1. **初始化管理员**
   首次访问会显示「初始化系统」页面：填用户名（至少 2 位）和密码（至少 6 位）。
   账号只保存在本机数据库里，忘记密码可在登录页重置（见 [常见问题](#常见问题)）。
2. **建客户**
   左侧「客户档案 → 新建客户」，填客户名称（编号选填）。
   进入客户详情后可以继续补：
   - 联系人（姓名必填，电话 / 邮箱 / 角色选填）
   - 网络信息版本（对接人、网关、IP 规划、VLAN、DNS、账号密码、带宽、拓扑说明……保存后自动生成版本和字段级变更记录）
   - 设备档案（型号、序列号、账号密码、拓扑图、网络信息）
3. **建项目**
   左侧「项目管理 → 新建项目」，选择客户、类型（新实施 / 加点 / 测试 / 维保）、状态、优先级、开始日期，按需添加产品线 / 具体产品。
   保存后会自动带出该类型的阶段模板，项目详情里可以改阶段状态、上传附件、记进度、提问题。
4. **记一条进度**
   项目详情 →「进度记录 → 新增进度」，填日期、当前进展、下一步计划（可关联多个阶段和附件）。
   首页看板会取最新一条的「下一步计划」。
5. **看一眼提醒**
   「提醒中心 → 立即扫描」，按维保 60 天 / 7 天未更新 / 验收前 3 天 / 14 天未结束生成提醒。
6. **改设置（可选）**
   「系统 / 备份」里可以改软件名称、提醒阈值、单个附件上限，以及配置自动备份。

到此为止就可以正常使用了。日常操作只有两步：**有进展就记一条进度，有问题就提一条风险问题**。

### 第 5 步：验证与停止

验证服务是否正常（浏览器打开也行）：

```bash
curl http://127.0.0.1:8787/api/state
# {"data":{"app_name":"项目管理","initialized":true,"subtitle":"...","version":"1.0.0"}}
```

停止：

- 前台运行：`Ctrl+C`（会优雅退出并关闭数据库）。
- systemd 托管：`sudo systemctl stop sangfor-pm`，见[服务器部署](#服务器部署systemd)。
- 不要直接 `kill -9`，虽然 SQLite 有 WAL 能恢复，但没必要冒险。

端到端自检（用临时数据目录，不碰你的正式数据）：

```bash
bash scripts/linux-smoke-test.sh ./sangfor-pm
```

它会依次验证：启动 → 初始化 → 建客户 → 建项目 → Excel 导出 → PDF 导出，结束后自动清理。

---

## 服务器部署（systemd）

完整步骤（防火墙、字体、时区、排错）见 **[docs/linux-deploy.md](docs/linux-deploy.md)**。最小流程：

```bash
# 1. 建运行账号与目录
sudo useradd --system --home /var/lib/sangfor-pm --shell /usr/sbin/nologin sangfor
sudo mkdir -p /opt/sangfor-pm /var/lib/sangfor-pm

# 2. 放二进制并安装 unit
sudo install -m 0755 sangfor-pm /opt/sangfor-pm/sangfor-pm
sudo cp deploy/sangfor-pm.service /etc/systemd/system/
sudo chown -R sangfor:sangfor /var/lib/sangfor-pm

# 3. 启动
sudo systemctl daemon-reload
sudo systemctl enable --now sangfor-pm
systemctl status sangfor-pm
curl -s http://127.0.0.1:8787/api/state
```

unit 里已经配置了 `-data /var/lib/sangfor-pm`、`TZ=Asia/Shanghai`、`UMask=0077` 以及基础加固。
升级只需停服务、替换 `/opt/sangfor-pm/sangfor-pm`、再启动。

---

## 局域网访问

程序默认监听 `0.0.0.0`，同一网段的其他主机用控制台打印的地址即可访问。

其他主机打不开，通常是防火墙拦了入站（端口按实际打印的改）：

```bash
# ufw（Ubuntu / Debian）
sudo ufw allow 8787/tcp

# firewalld（CentOS / RHEL / Fedora）
sudo firewall-cmd --add-port=8787/tcp --permanent && sudo firewall-cmd --reload
```

> ⚠️ 服务是 **HTTP 明文**，且附件与网络信息中的账号密码按需求明文存储。请只在**可信内网**开放，
> 公共网络下建议用 `-host 127.0.0.1` 收回本机，或放在 HTTPS 反向代理之后。

---

## 升级、迁移与卸载

### 升级

数据在数据目录里，升级只换二进制，**不会动数据**。仍建议先备份。

```bash
# 1. 停服务（Ctrl+C 或 systemctl stop）
# 2. 拉新代码（当前分支应为 For_Linux）
git pull
# 3. 重新编译
bash build.sh
# 4. 重新启动
./sangfor-pm
```

如果 `git pull` 提示本地有修改冲突（比如改过 README），先 `git stash` 再 pull，或先提交你的改动。

### 迁移 / 换机

1. 停掉旧机器上的服务；
2. 把整个数据目录拷贝到新机器（`data.db`、`files/`、`backups/`、`tmp/` 都要）；
3. 新机器上编译 / 拷贝二进制，用 `-data` 指向新目录启动；
4. 把目录所有者改成运行服务的用户：`sudo chown -R sangfor:sangfor <数据目录>`。

SQLite 使用 WAL 模式，**请在停服务后拷贝**，不要在运行中直接 cp 单个 `data.db`。

### 卸载

1. 停服务；
2. 删除二进制（`sangfor-pm`）和 systemd unit；
3. 确认不再需要数据后，删除数据目录（`~/.sangfor-pm` 或你 `-data` 指定的目录）。

删除数据目录等于清空所有客户 / 项目 / 附件，操作前先备份。

---

## 数据与备份

### 数据目录（系统固定，界面不可改）

| 方式 | 路径 |
|---|---|
| 默认 | `~/.sangfor-pm` |
| 命令行覆盖 | `-data /var/lib/sangfor-pm` |
| 环境变量覆盖 | `PM_DATA_DIR` |

优先级：`-data` > `PM_DATA_DIR` > 默认目录。

> 启动时会把数据目录权限收紧为 `0700`（库内含明文设备密码）。
> 服务器建议 `-data /var/lib/sangfor-pm` 并由 systemd 托管，见 [docs/linux-deploy.md](docs/linux-deploy.md)。

```
<数据目录>/
├─ data.db          SQLite 主库（WAL 模式）
├─ files/           附件、拓扑图等原始文件
├─ backups/         自动备份 zip
├─ tmp/             上传分片临时目录（启动时清理 24h 前残留）
└─ RESET_TOKEN.txt  仅在生成密码重置令牌时出现
```

**迁移 / 换机**：停服务后直接拷贝整个目录即可，数据库与附件都在里面。

### 本地自动备份

- 备份内容：数据库一致性快照（`VACUUM INTO`）+ 附件文件（可选）
- 频率：每 N 小时 / 天 / 周 / 月；保留份数可设，超出自动清理最旧的
- 后台每分钟检查一次是否到达下次备份时间
- 也可在「系统 / 备份」页手动「立即备份」并下载 zip

备份 zip 的结构：`data.db` + `files/...` + `backup-info.txt`，恢复时解压覆盖数据目录即可（先停服务）。

### 云端 S3

第一版仅保留配置项（Endpoint / Region / Bucket / AK / SK / 前缀），**尚未接入 SDK**，属于后续迭代。
按设计云端只做备份，不做多端同步。

---

## 目录结构

```
.
├─ main.go                      启动入口：数据目录、端口顺延、局域网地址、优雅退出、自动开浏览器
├─ build.sh                     Linux 构建脚本（CGO_ENABLED=0，支持交叉编译）
├─ start.sh                     一键启动（无二进制先编译；无桌面自动 -no-open）
├─ internal/
│  ├─ store/
│  │  ├─ schema.go              25 张表的建表 SQL（幂等）
│  │  └─ db.go                  连接、数据目录、种子数据、通用 CRUD（白名单列，防注入）
│  ├─ auth/auth.go              PBKDF2 口令哈希 + 内存会话 + 重置令牌
│  ├─ fonts/                    中文字体查找 + TTC 拆包 + cmap 中文校验
│  └─ httpapi/
│     ├─ server.go              路由表、中间件、登录限流
│     ├─ handlers_auth.go       初始化 / 登录 / 改密 / 重置 / 系统设置
│     ├─ handlers_customers.go  客户、联系人、网络版本、设备档案、上传
│     ├─ handlers_projects.go   项目、阶段、设备快照、网络引用、附件版本、维保
│     ├─ handlers_records.go    进度记录、风险问题、项目转化与继承
│     ├─ handlers_dashboard.go  看板、提醒引擎、全局搜索、元数据
│     ├─ handlers_export.go     Excel 六表 + PDF 四模板
│     └─ handlers_backup.go     zip 备份、清理、定时调度、批量改状态
├─ deploy/sangfor-pm.service    systemd unit 模板
├─ docs/linux-deploy.md         Linux 部署 / 字体 / systemd / 防火墙 / 排错
├─ scripts/linux-smoke-test.sh  Linux 冒烟测试（临时数据目录，跑完整流程）
└─ web/                         内嵌前端（无构建）
   ├─ index.html
   ├─ css/app.css               设计系统（浅色 / 深色双主题）
   └─ js/
      ├─ api.js                 接口封装
      ├─ ui.js                  弹窗 / 表单 / 表格 / 徽标
      ├─ app.js                 登录、导航、hash 路由、全局搜索
      └─ pages/                 home · customers · projects · records · misc
```

---

## 数据模型

25 张表，覆盖需求文档中的全部实体：

| 分组 | 表 |
|---|---|
| 账号与环境 | `users` `settings` |
| 客户域 | `customers` `contacts` `net_versions` `net_changes` `devices` `device_changes` |
| 项目域 | `projects` `project_products` `project_stages` `project_relations` `project_devices` `project_device_changes` `project_net_refs` |
| 附件 | `attachments` `attachment_versions` |
| 记录 | `progress` `progress_stages` `progress_attachments` `issues` |
| 提醒 / 维保 | `reminders` `maintenance` |
| 运维 | `backup_config` `export_logs` |

设计要点：

- **版本化**：网络信息按客户整体出版本，字段级差异写 `net_changes`；设备改动写 `*_changes`
- **快照隔离**：项目里的设备是客户设备档案的副本，改项目不动客户档案
- **附件继承**：转化项目时物理复制文件生成新版本，改新项目不影响原项目

---

## 安全说明

面向个人单用户场景，按需求做了明确的取舍，请知悉：

| 项 | 现状 |
|---|---|
| 账号密码（客户设备） | **明文存储、列表与详情直接展示、导出明文** —— 按需求实现 |
| 传输 | HTTP 明文，无 TLS。仅建议在可信内网使用 |
| 重置密码 | 一次性令牌（30 分钟有效），**仅允许在运行服务的本机生成**，令牌同时写入数据目录 `RESET_TOKEN.txt` |
| 登录防护 | 同 IP 连续 5 次密码错误锁定 60 秒 |
| 会话 | 内存会话，服务重启后需重新登录 |
| SQL | 全部参数绑定；写操作走字段白名单 |

---

## 已知限制与 Roadmap

**已知限制**

- 云端 S3 备份未实现（仅配置项）
- 备份只产出 zip，**没有一键恢复**（恢复 = 解压后替换数据目录）
- 单用户，无多账号 / 权限体系
- 附件为物理删除，无回收站；无跨项目去重
- 无日历视图（按需求第一版不做）
- SQLite 单连接串行写入，多端同时操作会排队（个人使用场景足够）

**Roadmap**

- [ ] 备份一键恢复（选 zip → 校验 → 回滚）
- [ ] S3 备份落地（附件 + 数据库快照）
- [ ] 附件回收站 / 去重（内容哈希）
- [ ] 项目列表自定义列与保存筛选视图
- [ ] 周报自动生成并归档历史周

---

## 常见问题

**`go: command not found` / `go.mod requires go >= 1.26.0`**
Go 没装或版本过低。按[第 0 步](#第-0-步准备环境)安装/升级 Go，**重开终端**后再 `go version` 确认。

**编译报 `Permission denied` / `$'\r': command not found`**
用 `bash build.sh` 而不是 `./build.sh`；CRLF 问题执行 `sed -i 's/\r$//' build.sh`。
仓库已通过 `.gitattributes` 固定 `*.sh` 为 LF，正常 clone 不会出现。

**`git clone` 很慢 / `TLS handshake` 失败**
换网络或代理；Go 模块慢可以设 `GOPROXY=https://goproxy.cn,direct`。
也可以直接在 GitHub 切到 `For_Linux` 分支下载 ZIP。

**提示端口被占用**
程序会自动顺延端口，控制台看实际地址；也可 `-port 9000` 指定。

**浏览器没有自动打开**
服务器 / 容器里没有桌面环境，属于正常；手动访问控制台打印的地址，或启动时加 `-no-open` 避免尝试。

**其他主机打不开**
先确认控制台打印的局域网地址与自己的网段一致，再放行防火墙（见[局域网访问](#局域网访问)）。虚拟机 / VPN / Docker 网卡的地址通常不可用。

**导出 PDF 报「未找到可用中文字体」**
PDF 库只支持 **TrueType（glyf）** 字体。`.ttc` 会被自动拆成单字体后加载，
但 **Noto Sans CJK / 思源黑体的 `.ttc` / `.otf` 是 CFF 轮廓，不能用于本项目的 PDF 导出**。
推荐安装（任选其一）：

```bash
sudo apt install fonts-wqy-zenhei fonts-wqy-microhei
sudo apt install fonts-arphic-uming fonts-droid-fallback
# 或手动指定任意 TrueType 中文字体
PM_PDF_FONT=/usr/share/fonts/truetype/wqy/wqy-zenhei.ttc ./sangfor-pm
```

程序会解析 cmap 校验字体确实包含「中」「文」，不会误选 DejaVu 这类无中文字形的字体；
选中的字体路径会打印在日志里。详见 [docs/linux-deploy.md](docs/linux-deploy.md#6-中文字体pdf-导出必需)。

**忘记密码**
登录页「忘记密码」→ 在**运行服务的本机**生成令牌 → 输入令牌与新密码。
也可直接看数据目录里的 `RESET_TOKEN.txt`。注意令牌 30 分钟过期，且只能在服务所在机器上生成。

**提示「打开数据库失败」/ 数据目录不可写**
检查 `-data` 指定的目录权限、磁盘空间；目录应为运行用户所有（`chown -R`）。
SELinux 环境执行 `restorecon -Rv <数据目录>`。

**时间 / 周报日期不对**
服务器时区常是 UTC。启动前设置 `TZ=Asia/Shanghai`，systemd 里写 `Environment=TZ=Asia/Shanghai`。

**上传大附件时内存暴涨 / `/tmp` 满**
程序默认把上传分片放到 `<数据目录>/tmp`，不会写 `/tmp`；如果手动设了 `TMPDIR` 就按你的设置走。
确认数据盘空间足够（单个附件上限 300MB）。

**把数据库放到 NFS / 共享盘后异常**
SQLite WAL 不建议放在网络文件系统上，请使用本地磁盘。

**想清空重来**
关掉服务，删除数据目录下的 `data.db*` 与 `files/`，重启后会重新引导初始化。

---

## 许可

[MIT](LICENSE) © 2026 yimu56

---

> 本项目为个人实施交付场景的自用工具，与深信服科技（Sangfor）官方无关，未使用其任何商标、代码或内部资料。
