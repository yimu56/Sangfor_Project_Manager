# Linux 部署与运行

本文面向把「项目管理」跑在 Linux（物理机 / 虚拟机 / 云主机 / NAS 容器）上的场景。
Windows 版本在 `main` 分支，本文件只讲 Linux。

程序是**单个静态二进制**：前端已 `embed` 进去，SQLite 用纯 Go 实现（`CGO_ENABLED=0`），
不依赖 Node、不需要安装数据库、不需要 C 编译器。

> **已在 Debian 13（x86_64，systemd）实测通过**：冒烟脚本、PDF 中文导出（文泉驿 `.ttc`）、
> 附件上传/下载（SHA256 一致）、含附件备份 zip、SIGTERM 优雅退出、systemd 托管。
> 无中文字体时 PDF 返回 501 并给出安装提示，不会静默导出缺字 PDF。

---

## 1. 最快路径：用预编译二进制

仓库外的 `dist/` 目录（不提交到 git）里如果已有构建产物，直接拷到服务器：

```bash
scp dist/sangfor-pm-linux-amd64 user@server:/opt/sangfor-pm/sangfor-pm
ssh user@server
sudo mkdir -p /opt/sangfor-pm /var/lib/sangfor-pm
sudo cp /tmp/sangfor-pm /opt/sangfor-pm/ && sudo chmod +x /opt/sangfor-pm/sangfor-pm
```

本机验证（前台跑）：

```bash
/opt/sangfor-pm/sangfor-pm -no-open -host 127.0.0.1 -port 8787
# 另开终端
curl -s http://127.0.0.1:8787/api/state
```

`-no-open` 表示不尝试打开浏览器；无桌面环境时程序也会自动跳过，但显式写上更稳。

---

## 2. 从源码构建

要求 **Go 1.26+**（`go.mod` 声明 `go 1.26.0`，`modernc.org/sqlite` 也要求 1.26）。

```bash
git clone <你的仓库> && cd Sangfor_Project_Manager
go version            # 必须 >= go1.26
bash build.sh         # 生成 ./sangfor-pm
# 或指定产物路径 / 架构
GOOS=linux GOARCH=arm64 OUT=dist/sangfor-pm-linux-arm64 bash build.sh
```

`build.sh` 已设置 `CGO_ENABLED=0`，可在任意平台交叉编译 Linux 版本（amd64 / arm64）。

> 如果 `./build.sh` 提示 `Permission denied`，用 `bash build.sh`，或先
> `chmod +x build.sh start.sh scripts/linux-smoke-test.sh`。
> 如果提示 `$'\r': command not found`，说明文件被转成了 CRLF，
> 执行 `sed -i 's/\r$//' build.sh` 或 `dos2unix build.sh`
> （仓库已用 `.gitattributes` 固定 `*.sh` 为 LF，正常 clone 不会出现）。

---

## 3. 数据目录

| 方式 | 路径 |
|---|---|
| 默认 | `~/.sangfor-pm` |
| 命令行指定 | `-data /var/lib/sangfor-pm` |
| 环境变量 | `PM_DATA_DIR=/var/lib/sangfor-pm` |

优先级：`-data` > `PM_DATA_DIR` > 默认目录。

目录结构：

```
<数据目录>/
├─ data.db          SQLite 主库（WAL 模式）
├─ files/           附件、拓扑图
├─ backups/         自动备份 zip
├─ tmp/             上传分片临时目录（启动时自动使用并清理 24 小时前残留）
└─ RESET_TOKEN.txt  仅在生成密码重置令牌时出现
```

**临时目录**：Linux 上 `/tmp` 常常是 tmpfs（占内存）。程序在 `TMPDIR` 未设置时
会把上传分片目录指到 `<数据目录>/tmp`，避免上传 300MB 附件时先吃掉 300MB 内存。
想用别的盘可自行 `export TMPDIR=/data/tmp`。

**权限**：启动时会把数据目录权限收紧为 `0700`（库里有明文设备密码）。
如果多个运维账号需要访问，请用同一个系统用户运行，不要 `chmod 777`。

**迁移**：停服务后整体拷贝目录即可；恢复同理。

**注意**：SQLite WAL 不建议放在 NFS / SMB 等网络文件系统上，请使用本地磁盘。

---

## 4. 用 systemd 托管（推荐）

1. 建运行账号与目录：

```bash
sudo useradd --system --home /var/lib/sangfor-pm --shell /usr/sbin/nologin sangfor
sudo mkdir -p /opt/sangfor-pm /var/lib/sangfor-pm
sudo cp sangfor-pm /opt/sangfor-pm/
sudo cp deploy/sangfor-pm.service /etc/systemd/system/
sudo chown -R sangfor:sangfor /var/lib/sangfor-pm
sudo chmod 755 /opt/sangfor-pm/sangfor-pm   # 必须 755：750 且属主 root 时 sangfor 用户无法执行
```

2. 按需修改 unit 里的端口、`-host`、时区，然后：

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now sangfor-pm
systemctl status sangfor-pm
journalctl -u sangfor-pm -f          # 查看日志
```

3. 升级：

```bash
sudo systemctl stop sangfor-pm
sudo cp sangfor-pm.new /opt/sangfor-pm/sangfor-pm && sudo chmod 755 /opt/sangfor-pm/sangfor-pm
sudo systemctl start sangfor-pm
```

unit 里已经做了基础加固（`ProtectSystem=strict`、`PrivateTmp`、`UMask=0077`、
`ReadWritePaths=/var/lib/sangfor-pm`）。如果不需要，可以整段删除。

> 有 SELinux 的发行版（CentOS / RHEL / Fedora）如果出现 `Permission denied`，
> 执行 `sudo restorecon -Rv /var/lib/sangfor-pm`，仍不行再检查 `audit.log` 里的 AVC。

---

## 5. 防火墙

程序默认监听 `0.0.0.0`，同网段可直接访问；被拦时放行 TCP 端口：

```bash
# ufw（Ubuntu / Debian）
sudo ufw allow 8787/tcp

# firewalld（CentOS / RHEL / Fedora）
sudo firewall-cmd --add-port=8787/tcp --permanent
sudo firewall-cmd --reload

# nftables
sudo nft add rule inet filter input tcp dport 8787 accept
```

只想本机使用：`-host 127.0.0.1`（systemd 里改 `ExecStart`）。
服务本身是 **HTTP 明文**，公网暴露请放在 Nginx / Caddy 的 HTTPS 反向代理之后，并加访问控制。

---

## 6. 中文字体（PDF 导出必需）

**这是 Linux 上最容易踩的坑**，原因：

- PDF 库 `gopdf` 只支持 **TrueType（glyf 轮廓）** 字体；
- `.ttc` 字体集合本身不能直接加载（程序会自动拆出其中一张字面，这点已处理）；
- **Noto Sans CJK / 思源黑体的 `.ttc` / `.otf` 是 CFF 轮廓，不支持**，
  即使装上 `fonts-noto-cjk` 也不能用于 PDF 导出。

程序按以下顺序查找：`PM_PDF_FONT` → 系统常见路径 → `fc-list :lang=zh` → 字体目录扫描，
并且会解析 `cmap` 确认字体真的包含「中」「文」两个字，避免选中 DejaVu 这类无中文字形的字体。

推荐安装（任选其一，都是 TrueType）：

```bash
# Debian / Ubuntu
sudo apt install fonts-wqy-zenhei fonts-wqy-microhei   # .ttc，程序自动拆
sudo apt install fonts-arphic-uming fonts-arphic-ukai
sudo apt install fonts-droid-fallback                  # DroidSansFallbackFull.ttf

# CentOS / RHEL / Fedora
sudo dnf install wqy-zenhei-fonts wqy-microhei-fonts
sudo dnf install google-droid-sans-fonts

# Arch
sudo pacman -S wqy-zenhei wqy-microhei
```

安装后刷新缓存（可选）：

```bash
fc-cache -fv
fc-list :lang=zh | head
```

手动指定（最稳）：

```bash
PM_PDF_FONT=/usr/share/fonts/truetype/wqy/wqy-zenhei.ttc ./sangfor-pm
# 或
./sangfor-pm -data /var/lib/sangfor-pm   # 配合 export PM_PDF_FONT=...
```

验证：

```bash
curl -s -o /tmp/t.pdf -w '%{http_code} %{size_download}\n' \
  -b cookies.txt 'http://127.0.0.1:8787/api/export/pdf?type=weekly&project_id=1'
head -c 4 /tmp/t.pdf       # 应为 %PDF
```

若返回 `501`，响应体会给出缺字体的提示；程序日志里会打印最终选中的字体路径
（`journalctl -u sangfor-pm | grep PDF`）。

---

## 7. 时区

程序内部用 `time.Local` 计算日期（周报、到期、提醒阈值），
容器 / 云主机常默认 UTC，会让「今天」和「本周」差 8 小时。建议：

```bash
# systemd
Environment=TZ=Asia/Shanghai

# 临时
TZ=Asia/Shanghai ./sangfor-pm
```

---

## 8. 冒烟测试

不碰正式数据，用临时目录跑完整流程：

```bash
bash scripts/linux-smoke-test.sh ./sangfor-pm
```

会验证：服务启动 → 初始化账号 → 建客户 → 建项目 → Excel 导出 → PDF 导出，
结束后自动清理临时目录。

---

## 9. 常见问题

**启动即退出，提示「打开数据库失败」**
数据目录不可写。检查 `-data` 路径权限 / 磁盘空间；SELinux 见上文。

**端口被占用**
程序会自动从 8787 往后试 20 个端口，控制台会打印实际地址；也可 `-port 9000` 指定。

**局域网打不开**
先确认控制台打印的地址与客户端同网段（虚拟网卡 / Docker / VPN 地址通常不可用），再放行防火墙。

**忘记密码**
在**运行服务的本机**（或 `ssh -L` 端口转发后从本机）打开登录页 →「忘记密码」生成一次性令牌；
也可以直接看数据目录的 `RESET_TOKEN.txt`。

**导出 PDF 中文是空白 / 报 501**
见第 6 节。确认安装的是 TrueType 中文字体，或用 `PM_PDF_FONT` 指定 `.ttf` / `.ttc`。

**想清空重来**
停服务，删除数据目录下的 `data.db*` 与 `files/`，重启后会重新引导初始化。

**容器里跑**
```bash
docker run -d --name pm -p 8787:8787 \
  -v pm-data:/data \
  -e TZ=Asia/Shanghai \
  -e PM_DATA_DIR=/data \
  <image> -no-open -host 0.0.0.0
```
（镜像需自行构建：`FROM scratch` + 二进制 + 时区/字体文件，或基于 `debian:stable-slim` 安装字体。）

---

## 10. 分支说明

本分支（`For_Linux`）只包含 Linux 版本；Windows 版本在 `main` 分支。
两者数据库结构一致，迁移时停服务后整体拷贝数据目录即可。
