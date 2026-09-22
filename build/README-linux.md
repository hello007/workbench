# WorkBench Linux 版

随发行包（`workbench-linux-amd64.tar.gz`）附带，扁平布局：`workbench` 二进制 + 本说明。

## 运行依赖

| 运行形态 | 依赖 |
| --- | --- |
| 浏览器访问模式（`--serve`） | 无（纯 Go HTTP/WebSocket 服务） |
| 桌面模式（GTK 窗口） | `libgtk-3`、`libwebkit2gtk-4.1` 等（glibc 基线 Ubuntu 22.04 / Debian 12） |

Debian/Ubuntu 桌面依赖安装：

```bash
sudo apt-get install -y libgtk-3 libwebkit2gtk-4.1
```

## 运行

```bash
./workbench --serve    # 无头服务（浏览器访问，令牌见 data/web_token）
./workbench            # 桌面模式（需图形会话；WSL2 需 WSLg）
```

监听地址、访问令牌与远程访问安全建议详见项目 `docs/部署说明.md` 第 4 章（跨平台一致）。
