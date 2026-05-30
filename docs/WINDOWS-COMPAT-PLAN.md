# daily-report-daemon Windows 平台兼容方案

版本：v0.1
日期：2026-05-30

## 1. 兼容目标

将 daily-report-daemon 从 macOS/Linux 无损迁移到 Windows，保证全部功能（CLI 扫描、Agent 引擎、Daemon 常驻、钉钉推送、团队聚合、Web UI）在 Windows 平台上正常工作。

## 2. 差异分析

### 2.1 进程管理（最大差异）

| 功能 | Unix 实现 | Windows 实现 | 改动量 |
|------|----------|-------------|--------|
| Daemon 启动 | fork + 后台进程 | Windows Service 或 后台进程 + 托盘 | 中 |
| 停止信号 | SIGTERM / SIGINT | Windows Service Control / 命名管道 / 文件锁 | 中 |
| PID 管理 | PID 文件 | Windows Service 状态 / 进程名检测 | 低 |
| 开机自启 | systemd / launchd | Windows Service 自动启动 / 注册表 Run | 中 |

**方案**：daemon 包新增 `internal/daemon/platform_windows.go`，使用 Windows Service API（`golang.org/x/sys/windows/svc`）实现 start/stop/status。备选：简单文件锁 + 轮询模式（不依赖 Service API）。

### 2.2 路径处理

| 场景 | 风险 | 改动 |
|------|------|------|
| 硬编码 `/` 分隔符 | 路径解析失败 | 全部替换为 `filepath.Join` / `filepath.Separator` |
| `~` 展开 | 找不到 home 目录 | 替换为 `os.UserHomeDir()` |
| 路径长度 > 260 字符 | Windows API 限制 | 使用 `\\?\` 前缀或启用长路径支持 |
| 配置文件位置 | `~/.daily-report-daemon/` | `%APPDATA%/daily-report-daemon/` |
| 大小写敏感 | macOS 默认不敏感，Windows 也不敏感 | 无影响 |

### 2.3 Git CLI 调用

| 场景 | 风险 | 改动 |
|------|------|------|
| 命令名 | `git` vs `git.exe` | Go `exec.Command` 自动查找 |
| 路径参数 | 需要 Windows 风格路径 | `filepath` 自动处理 |
| 输出编码 | Windows 中文版输出 GBK | 设置 `LANG=en_US.UTF-8` 环境变量或检测编码 |
| diff 换行符 | CRLF vs LF | git 配置 `core.autocrlf` 可能影响 diff，需标准化 |

### 2.4 文件系统

| 场景 | 风险 | 改动 |
|------|------|------|
| 文件权限 | Unix 权限模型 vs Windows ACL | 扫描阶段无影响，不修改文件 |
| 隐藏文件 | `.xxx` vs 文件属性 | `.gitignore` 语义一致 |
| 二进制检测 | MIME type 检测 | 已有扩展名过滤，够用 |
| fsnotify | inotify vs ReadDirectoryChangesW | go fsnotify 已支持 Windows，需验证 |

### 2.5 Shell 脚本

| 脚本 | Unix | Windows |
|------|------|--------|
| `smoke-test.sh` | bash | 新增 `smoke-test.bat` |
| Makefile | GNU Make | 新增 `make.bat` 或直接 `go build` |
| `make build/test/install` | Make | 保留 go 命令备用 |

### 2.6 其他

| 场景 | 改动 |
|------|------|
| 二进制扩展名 | 构建输出 `daily-report-daemon.exe` |
| 路径遍历保护 | `filepath.Clean` + 盘符检测 |
| Token 存储加密 | AES-256-GCM 不依赖平台 API，无影响 |
| Web UI | HTTP server 跨平台，无影响 |

## 3. 实施计划

### M1：构建系统适配（0.5 天）

- [ ] Go build tag 区分平台
- [ ] Makefile + `.bat` 替代脚本
- [ ] CI 增加 Windows 构建目标
- [ ] `.exe` 扩展名处理

### M2：路径与配置兼容（0.5 天）

- [ ] 全局审计：替换所有硬编码 `/` 为 `filepath.Join`
- [ ] 配置文件位置：`os.UserConfigDir()` → `%APPDATA%`
- [ ] 运行时目录：`os.UserHomeDir()` 适配
- [ ] 长路径支持：`\\?\` 前缀

### M3：Daemon 进程管理（1-2 天）

- [ ] 实现 `platform_windows.go`：
  - Windows Service 注册/启动/停止
  - 备选：文件锁 + 后台进程模式
- [ ] daemon 命令 Windows 适配
- [ ] 系统托盘通知（可选，`systray` 库）

### M4：Git 与编码兼容（0.5 天）

- [ ] Git 输出编码检测（UTF-8 / GBK）
- [ ] CRLF 换行符标准化
- [ ] Windows Git 路径格式验证

### M5：集成测试与文档（0.5 天）

- [ ] Windows 环境端到端测试
- [ ] 更新用户手册 Windows 章节
- [ ] 安装包构建（MSI / 绿色版 zip）

**总工期**：3-4 天

## 4. 架构设计

### 4.1 平台抽象层

```
internal/daemon/
  daemon.go           # 通用接口
  platform_unix.go    # Unix: signal + PID file
  platform_windows.go # Windows: Service API / 文件锁
```

Go build tags：
```go
//go:build windows
// +build windows

//go:build !windows
// +build !windows
```

### 4.2 Daemon 管理方案（推荐）

优先使用 **文件锁 + 后台进程**（轻量，不依赖 Windows Service API），备选 Windows Service。

```
# 启动
daily-report-daemon.exe daemon start
→ 检查 daemon.lock 文件是否存在
→ 不存在：创建 lock + 启动后台 goroutine
→ 存在：提示 "daemon already running"

# 停止
daily-report-daemon.exe daemon stop  
→ 写入 daemon.lock 中的 stop 标记
→ 后台进程检测到标记后优雅退出
→ 删除 lock 文件

# 状态
daily-report-daemon.exe daemon status
→ 检查 daemon.lock 存在 + 进程是否存活
```

### 4.3 路径抽象

```go
func configDir() string {
    dir, _ := os.UserConfigDir()  // %APPDATA% on Windows
    return filepath.Join(dir, "daily-report-daemon")
}
```

## 5. 风险与应对

| 风险 | 影响 | 应对 |
|------|------|------|
| Windows Git 中文 GBK 编码 | diff 解析乱码 | 设置 git 环境变量 `LC_ALL=en_US.UTF-8` 或检测编码自动转换 |
| Windows Service 权限 | 普通用户无法注册服务 | 优先用文件锁方案，Service 作为高级选项 |
| 路径长度超限 | 深层目录扫描失败 | 使用 `\\?\` 长路径前缀 + 跳过超长路径并记录日志 |
| 杀毒软件误报 | 二进制被拦截 | Go 静态编译，添加数字签名 |
| 没有 Go 环境 | 用户无法编译 | 提供预编译 `.exe` 下载 |

## 6. 不做的

- 不做 UWP/Store 应用
- 不做 MSI 安装包（Phase 4，先绿色版 zip）
- 不做 Windows 7 兼容（Go 1.21+ 最低 Win10/Server 2016）
