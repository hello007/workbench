"""Git Manager CLI — Click 命令组 + REPL 交互模式。"""

import json
import os
import sys

import click

from cli_anything.gitmanager.core import project, gitops, fileops, session


class SessionContext:
    """Click 上下文中保存的会话状态。"""

    def __init__(self):
        self.session = session.Session()
        self.config = project.DirectoryConfig()
        self.json_output = False
        self.project_path = None


pass_session = click.make_pass_decorator(SessionContext, ensure=True)


def _out(ctx: SessionContext, data, human_func=None):
    """统一输出：--json 模式输出 JSON，否则调用人类可读函数。"""
    if ctx.json_output:
        click.echo(json.dumps(data, ensure_ascii=False, indent=2))
    elif human_func:
        human_func()
    else:
        click.echo(data)


@click.group(invoke_without_command=True)
@click.option("--json", "json_output", is_flag=True, help="JSON 格式输出")
@click.option("--project", "project_path", help="指定项目配置路径")
@click.pass_context
def cli(ctx, json_output, project_path):
    """Git Manager CLI — Git 仓库管理命令行工具。"""
    ctx.ensure_object(SessionContext)
    ctx.obj.json_output = json_output
    ctx.obj.project_path = project_path
    if project_path:
        ctx.obj.config = project.DirectoryConfig(project_path)
    ctx.obj.session.load()

    if ctx.invoked_subcommand is None:
        ctx.invoke(repl_cmd)


# ─── directory 命令组 ───────────────────────────────────────────────

@cli.group("directory", short_help="工作目录管理")
def directory():
    """管理工作目录配置。"""
    pass


@directory.command("list")
@pass_session
def dir_list(ctx):
    """列出所有工作目录。"""
    dirs = ctx.config.list_all()

    def human():
        if not dirs:
            click.echo("暂无工作目录")
            return
        for d in dirs:
            default = " [默认]" if d.get("isDefault") else ""
            click.echo(f"  {d['id'][:16]}...  {d['name']}  {d['path']}{default}")

    _out(ctx, dirs, human)


@directory.command("add")
@click.argument("name")
@click.argument("path")
@click.option("--default", "is_default", is_flag=True, help="设为默认目录")
@pass_session
def dir_add(ctx, name, path, is_default):
    """添加工作目录。"""
    try:
        d = ctx.config.add(name, path, is_default)
        _out(ctx, d, lambda: click.echo(f"已添加: {d['name']} ({d['path']})"))
    except (FileNotFoundError, ValueError) as e:
        click.echo(f"错误: {e}", err=True)
        sys.exit(1)


@directory.command("remove")
@click.argument("dir_id")
@pass_session
def dir_remove(ctx, dir_id):
    """删除工作目录。"""
    try:
        ctx.config.remove(dir_id)
        _out(ctx, {"removed": dir_id}, lambda: click.echo(f"已删除: {dir_id}"))
    except ValueError as e:
        click.echo(f"错误: {e}", err=True)
        sys.exit(1)


@directory.command("default")
@click.argument("dir_id", required=False)
@pass_session
def dir_default(ctx, dir_id):
    """设置或查看默认工作目录。"""
    if dir_id:
        try:
            ctx.config.set_default(dir_id)
            _out(ctx, {"defaultId": dir_id}, lambda: click.echo(f"默认目录已设置: {dir_id}"))
        except ValueError as e:
            click.echo(f"错误: {e}", err=True)
            sys.exit(1)
    else:
        d = ctx.config.get_default()
        _out(ctx, d, lambda: click.echo(f"{d['name']} ({d['path']})") if d else click.echo("未设置默认目录"))


@directory.command("use")
@click.argument("dir_id")
@pass_session
def dir_use(ctx, dir_id):
    """切换到指定工作目录（REPL 会话）。"""
    d = ctx.config.get(dir_id)
    if not d:
        click.echo(f"错误: 工作目录不存在: {dir_id}", err=True)
        sys.exit(1)
    ctx.session.set_directory(d["id"], d["path"])
    ctx.session.save()
    _out(ctx, d, lambda: click.echo(f"已切换到: {d['name']} ({d['path']})"))


# ─── git 命令组 ──────────────────────────────────────────────────────

@cli.group("git", short_help="Git 仓库操作")
def git():
    """Git 仓库操作命令。"""
    pass


@git.command("info")
@click.argument("path", required=False)
@pass_session
def git_info(ctx, path):
    """查看仓库信息。"""
    path = path or ctx.session.current_path or "."
    try:
        info = gitops.get_info(path)
        _out(ctx, info, lambda: _print_info(info))
    except Exception as e:
        click.echo(f"错误: {e}", err=True)
        sys.exit(1)


@git.command("clone")
@click.argument("url")
@click.argument("path", required=False)
@pass_session
def git_clone(ctx, url, path):
    """克隆仓库。"""
    path = path or "."
    try:
        output = gitops.clone(url, path)
        _out(ctx, {"url": url, "path": path, "output": output},
             lambda: click.echo(f"克隆完成: {url} -> {path}"))
    except (FileExistsError, RuntimeError) as e:
        click.echo(f"错误: {e}", err=True)
        sys.exit(1)


@git.command("pull")
@click.argument("path", required=False)
@pass_session
def git_pull(ctx, path):
    """拉取更新。"""
    path = path or ctx.session.current_path or "."
    try:
        output = gitops.pull(path)
        _out(ctx, {"path": path, "output": output}, lambda: click.echo(output or "已是最新"))
    except (ValueError, RuntimeError) as e:
        click.echo(f"错误: {e}", err=True)
        sys.exit(1)


@git.command("branches")
@click.argument("path", required=False)
@pass_session
def git_branches(ctx, path):
    """查看分支列表。"""
    path = path or ctx.session.current_path or "."
    try:
        branches = gitops.get_branches(path)

        def human():
            for b in branches:
                prefix = "* " if b["isCurrent"] else "  "
                remote = " (remote)" if b["isRemote"] else ""
                click.echo(f"{prefix}{b['name']}{remote}")

        _out(ctx, branches, human)
    except (ValueError, RuntimeError) as e:
        click.echo(f"错误: {e}", err=True)
        sys.exit(1)


@git.command("checkout")
@click.argument("branch")
@click.argument("path", required=False)
@click.option("--remote", "is_remote", is_flag=True, help="从远程分支创建本地跟踪分支")
@pass_session
def git_checkout(ctx, branch, path, is_remote):
    """切换分支。"""
    path = path or ctx.session.current_path or "."
    try:
        output = gitops.checkout(path, branch, is_remote)
        _out(ctx, {"branch": branch, "output": output},
             lambda: click.echo(f"已切换到分支: {branch}"))
    except (ValueError, RuntimeError) as e:
        click.echo(f"错误: {e}", err=True)
        sys.exit(1)


@git.command("log")
@click.argument("path", required=False)
@click.option("--limit", "-n", default=20, help="提交数量")
@click.option("--offset", default=0, help="偏移量")
@pass_session
def git_log(ctx, path, limit, offset):
    """查看提交历史。"""
    path = path or ctx.session.current_path or "."
    try:
        commits = gitops.get_log(path, limit, offset)

        def human():
            for c in commits:
                click.echo(f"  {c['shortSHA']}  {c['author']}  {c['message']}")

        _out(ctx, commits, human)
    except (ValueError, RuntimeError) as e:
        click.echo(f"错误: {e}", err=True)
        sys.exit(1)


@git.command("status")
@click.argument("path", required=False)
@pass_session
def git_status(ctx, path):
    """查看本地变更。"""
    path = path or ctx.session.current_path or "."
    try:
        changes = gitops.get_status(path)

        def human():
            if not changes:
                click.echo("工作目录干净")
                return
            for c in changes:
                click.echo(f"  {c['status']:>2}  {c['path']}")

        _out(ctx, changes, human)
    except (ValueError, RuntimeError) as e:
        click.echo(f"错误: {e}", err=True)
        sys.exit(1)


# ─── file 命令组 ─────────────────────────────────────────────────────

@cli.group("file", short_help="文件/文件夹操作")
def file():
    """文件和文件夹操作。"""
    pass


@file.command("tree")
@click.argument("path", required=False)
@click.option("--depth", "-d", default=-1, help="最大深度，-1 为不限制")
@pass_session
def file_tree(ctx, path, depth):
    """浏览文件树。"""
    path = path or ctx.session.current_path or "."
    try:
        tree = fileops.get_tree(path, max_depth=depth)
        _out(ctx, tree, lambda: _print_tree(tree))
    except NotADirectoryError as e:
        click.echo(f"错误: {e}", err=True)
        sys.exit(1)


@file.command("preview")
@click.argument("filepath")
@click.option("--max-size", default=1048576, help="最大预览大小（字节）")
@pass_session
def file_preview(ctx, filepath, max_size):
    """预览文件内容。"""
    try:
        result = fileops.preview_file(filepath, max_size)
        _out(ctx, result, lambda: _print_preview(result))
    except Exception as e:
        click.echo(f"错误: {e}", err=True)
        sys.exit(1)


@file.command("create")
@click.argument("parent")
@click.argument("name")
@click.option("--content", default="", help="文件内容")
@pass_session
def file_create(ctx, parent, name, content):
    """创建文件。"""
    try:
        path = fileops.create_file(parent, name, content)
        _out(ctx, {"path": path}, lambda: click.echo(f"已创建: {path}"))
    except FileExistsError as e:
        click.echo(f"错误: {e}", err=True)
        sys.exit(1)


@file.command("mkdir")
@click.argument("parent")
@click.argument("name")
@pass_session
def file_mkdir(ctx, parent, name):
    """创建文件夹。"""
    try:
        path = fileops.create_directory(parent, name)
        _out(ctx, {"path": path}, lambda: click.echo(f"已创建: {path}"))
    except FileExistsError as e:
        click.echo(f"错误: {e}", err=True)
        sys.exit(1)


@file.command("rename")
@click.argument("path")
@click.argument("new_name")
@pass_session
def file_rename(ctx, path, new_name):
    """重命名文件或文件夹。"""
    try:
        new_path = fileops.rename(path, new_name)
        _out(ctx, {"oldPath": path, "newPath": new_path},
             lambda: click.echo(f"已重命名: {path} -> {new_path}"))
    except (FileNotFoundError, FileExistsError) as e:
        click.echo(f"错误: {e}", err=True)
        sys.exit(1)


@file.command("delete")
@click.argument("path")
@click.option("--yes", "-y", is_flag=True, help="跳过确认")
@pass_session
def file_delete(ctx, path, yes):
    """删除文件或文件夹。"""
    if not yes and not click.confirm(f"确认删除 {path}?"):
        return
    try:
        fileops.delete(path)
        _out(ctx, {"deleted": path}, lambda: click.echo(f"已删除: {path}"))
    except FileNotFoundError as e:
        click.echo(f"错误: {e}", err=True)
        sys.exit(1)


# ─── open 命令组 ─────────────────────────────────────────────────────

@cli.group("open", short_help="外部程序打开")
def open_group():
    """用外部程序打开路径。"""
    pass


@open_group.command("explorer")
@click.argument("path", required=False)
@pass_session
def open_explorer(ctx, path):
    """在资源管理器中打开。"""
    path = path or ctx.session.current_path or "."
    from cli_anything.gitmanager.utils import backend
    try:
        backend.open_in_explorer(path)
        _out(ctx, {"path": path, "opened": "explorer"}, lambda: click.echo(f"已在资源管理器打开: {path}"))
    except FileNotFoundError as e:
        click.echo(f"错误: {e}", err=True)
        sys.exit(1)


@open_group.command("vscode")
@click.argument("path", required=False)
@pass_session
def open_vscode(ctx, path):
    """用 VSCode 打开。"""
    path = path or ctx.session.current_path or "."
    from cli_anything.gitmanager.utils import backend
    try:
        backend.open_in_vscode(path)
        _out(ctx, {"path": path, "opened": "vscode"}, lambda: click.echo(f"已用 VSCode 打开: {path}"))
    except RuntimeError as e:
        click.echo(f"错误: {e}", err=True)
        sys.exit(1)


# ─── batch 命令组 ────────────────────────────────────────────────────

@cli.group("batch", short_help="批量操作")
def batch():
    """批量操作多个仓库。"""
    pass


@batch.command("pull")
@click.argument("dir_path")
@pass_session
def batch_pull(ctx, dir_path):
    """批量拉取目录下所有 Git 仓库。"""
    repos = gitops.scan_git_repos(dir_path)
    if not repos:
        click.echo("未找到 Git 仓库")
        return
    results = []
    for repo in repos:
        try:
            output = gitops.pull(repo)
            results.append({"path": repo, "success": True, "output": output})
            click.echo(f"  ✓ {repo}")
        except Exception as e:
            results.append({"path": repo, "success": False, "error": str(e)})
            click.echo(f"  ✗ {repo}: {e}")
    _out(ctx, results)


@batch.command("status")
@click.argument("dir_path")
@pass_session
def batch_status(ctx, dir_path):
    """批量查看仓库状态。"""
    repos = gitops.scan_git_repos(dir_path)
    if not repos:
        click.echo("未找到 Git 仓库")
        return
    results = []
    for repo in repos:
        try:
            info = gitops.get_info(repo)
            changes = gitops.get_status(repo)
            results.append({**info, "changes": len(changes)})
            click.echo(f"  {os.path.basename(repo):20} {info.get('branch', '?'):15} 变更: {len(changes)}")
        except Exception as e:
            results.append({"path": repo, "error": str(e)})
            click.echo(f"  {os.path.basename(repo):20} 错误: {e}")
    _out(ctx, results)


# ─── session 命令组 ──────────────────────────────────────────────────

@cli.group("session", short_help="会话管理")
def session_cmd():
    """管理 REPL 会话状态。"""
    pass


@session_cmd.command("status")
@pass_session
def session_status(ctx):
    """查看当前会话状态。"""
    _out(ctx, ctx.session.to_dict(),
         lambda: _print_session(ctx.session))


@session_cmd.command("save")
@click.argument("name", default="default")
@pass_session
def session_save(ctx, name):
    """保存会话。"""
    path = ctx.session.save(name)
    _out(ctx, {"saved": path}, lambda: click.echo(f"会话已保存: {path}"))


@session_cmd.command("load")
@click.argument("name", default="default")
@pass_session
def session_load(ctx, name):
    """加载会话。"""
    if ctx.session.load(name):
        _out(ctx, ctx.session.to_dict(), lambda: click.echo("会话已加载"))
    else:
        click.echo(f"会话不存在: {name}", err=True)
        sys.exit(1)


@session_cmd.command("undo")
@pass_session
def session_undo(ctx):
    """撤销上一步操作。"""
    action = ctx.session.undo()
    if action:
        _out(ctx, action, lambda: click.echo(f"已撤销: {action.get('type', '?')}"))
    else:
        click.echo("无可撤销操作")


@session_cmd.command("redo")
@pass_session
def session_redo(ctx):
    """重做操作。"""
    action = ctx.session.redo()
    if action:
        _out(ctx, action, lambda: click.echo(f"已重做: {action.get('type', '?')}"))
    else:
        click.echo("无可重做操作")


# ─── REPL ─────────────────────────────────────────────────────────────

@cli.command("repl", hidden=True)
@pass_session
def repl_cmd(ctx):
    """交互式 REPL 模式。"""
    click.echo("╔══════════════════════════════════════╗")
    click.echo("║     Git Manager CLI v0.1.0           ║")
    click.echo("║     输入 help 查看可用命令           ║")
    click.echo("╚══════════════════════════════════════╝")
    click.echo()

    if ctx.session.current_directory_path:
        click.echo(f"当前目录: {ctx.session.current_directory_path}")
    click.echo()

    while True:
        try:
            prompt = "git-manager> "
            line = input(prompt).strip()
        except (EOFError, KeyboardInterrupt):
            click.echo("\n再见!")
            break

        if not line:
            continue
        if line.lower() in ("exit", "quit", "q"):
            ctx.session.save()
            click.echo("再见!")
            break
        if line.lower() == "help":
            _print_help()
            continue

        try:
            cli.main(line.split(), standalone_mode=False,
                     obj=ctx, parent=None)
        except SystemExit:
            pass
        except click.UsageError as e:
            click.echo(f"用法错误: {e}")
        except Exception as e:
            click.echo(f"错误: {e}")


# ─── 辅助函数 ─────────────────────────────────────────────────────────

def _print_info(info: dict):
    if not info.get("isRepo"):
        click.echo(f"  {info['path']}: 不是 Git 仓库")
        return
    click.echo(f"  路径:   {info['path']}")
    click.echo(f"  分支:   {info.get('branch', '-')}")
    click.echo(f"  远程:   {info.get('remote', '-')} {info.get('remoteUrl', '')}")


def _print_tree(nodes: list[dict], prefix: str = ""):
    for i, node in enumerate(nodes):
        is_last = i == len(nodes) - 1
        connector = "└── " if is_last else "├── "
        git_marker = " [GIT]" if node.get("isGitRepo") else ""
        click.echo(f"{prefix}{connector}{node['name']}{git_marker}")
        if "children" in node:
            ext = "    " if is_last else "│   "
            _print_tree(node["children"], prefix + ext)


def _print_preview(result: dict):
    click.echo(f"  文件: {result['name']}  ({result.get('size', 0)} bytes)")
    if result.get("isBinary"):
        click.echo("  [二进制文件]")
    elif result.get("tooLarge"):
        click.echo("  [文件过大]")
    elif result.get("content"):
        click.echo(result["content"][:500])
        if len(result["content"]) > 500:
            click.echo("  ... (已截断)")


def _print_session(s: session.Session):
    if s.current_directory_path:
        click.echo(f"  工作目录: {s.current_directory_path}")
    if s.current_path:
        click.echo(f"  当前路径: {s.current_path}")
    click.echo(f"  撤销栈: {len(s._undo_stack)}  重做栈: {len(s._redo_stack)}")


def _print_help():
    click.echo("""
可用命令:
  directory list              列出工作目录
  directory add <name> <path> 添加工作目录
  directory use <id>          切换工作目录
  directory default [id]      查看/设置默认目录

  git info [path]             查看仓库信息
  git pull [path]             拉取更新
  git branches [path]         查看分支
  git checkout <branch> [path] 切换分支
  git log [path]              提交历史
  git status [path]           本地变更

  file tree [path]            文件树
  file preview <path>         预览文件
  file create <parent> <name> 创建文件
  file mkdir <parent> <name>  创建文件夹
  file rename <path> <name>   重命名
  file delete <path>          删除

  open explorer [path]        资源管理器打开
  open vscode [path]          VSCode 打开

  batch pull <dir>            批量拉取
  batch status <dir>          批量状态

  session status              会话状态
  session save [name]         保存会话

  exit / quit                 退出 REPL
""")
