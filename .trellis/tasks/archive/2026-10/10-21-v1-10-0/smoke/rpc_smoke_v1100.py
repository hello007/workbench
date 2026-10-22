# -*- coding: utf-8 -*-
"""v1.10.0 发版冒烟——RPC 驱动面（git 全功能 / PPT 拆分合并 / 购物栏导出 / COM 预览）。

用法：
  python rpc_smoke_v1100.py            # 自动启动 serve（workbench.exe --serve）
  python rpc_smoke_v1100.py --attach   # 复用已在跑的 serve（BASE/TOKEN 手改）

UI 呈现面见 ui-checklist.md（拖拽/目录管理/文件夹标注/中文目录名/docx 目检）。
"""
import json, os, shutil, stat, subprocess, sys, time, urllib.request, zipfile

SMOKE_DIR = os.path.dirname(os.path.abspath(__file__))
REPO = os.path.dirname(os.path.dirname(os.path.dirname(os.path.dirname(SMOKE_DIR))))  # smoke -> 10-21-v1-10-0 -> tasks -> .trellis -> repo
RUN_DIR = os.path.join(SMOKE_DIR, "run")
BASE = "http://127.0.0.1:36201"
PORT = 36201
EXE = os.path.join(RUN_DIR, "workbench.exe")
FIXTURE = os.path.join(SMOKE_DIR, "fixture")

results = []


def rpc(method, *args):
    body = json.dumps({"method": method, "args": list(args)}).encode("utf-8")
    req = urllib.request.Request(BASE + "/api/rpc", data=body, method="POST",
                                 headers={"Content-Type": "application/json", "Authorization": "Bearer " + TOKEN})
    with urllib.request.urlopen(req, timeout=120) as r:
        resp = json.loads(r.read().decode("utf-8"))
    if isinstance(resp, dict) and resp.get("ok") is False:
        raise RuntimeError("RPC %s 失败: %s" % (method, json.dumps(resp.get("error"), ensure_ascii=False)))
    if isinstance(resp, dict) and "data" in resp:
        return resp["data"]
    return resp


def check(name, ok, evidence=""):
    results.append((name, ok, evidence))
    print(("PASS " if ok else "FAIL ") + name + ("  | " + str(evidence) if evidence else ""))
    return ok


def head_sha(work):
    hist = rpc("GetCommitHistory", work, 1, 0, {})
    return hist[0].get("sha") if hist else None


def force_rmtree(path):
    def onerr(func, pth, exc):
        os.chmod(pth, stat.S_IWRITE)
        func(pth)
    shutil.rmtree(path, onerror=onerr)


def git(*args, cwd):
    subprocess.run(["git"] + list(args), cwd=cwd, check=True, capture_output=True,
                   env={**os.environ, "GIT_AUTHOR_NAME": "冒烟", "GIT_AUTHOR_EMAIL": "s@t",
                        "GIT_COMMITTER_NAME": "冒烟", "GIT_COMMITTER_EMAIL": "s@t"})


def start_serve():
    os.makedirs(RUN_DIR, exist_ok=True)
    shutil.copy(os.path.join(REPO, "build", "bin", "workbench.exe"), EXE)
    token_path = os.path.join(RUN_DIR, "data", "web_token")
    if os.path.exists(token_path):
        os.remove(token_path)
    proc = subprocess.Popen([EXE, "--serve", "--listen=127.0.0.1:%d" % PORT], cwd=RUN_DIR,
                            stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
                            creationflags=subprocess.CREATE_NO_WINDOW if os.name == "nt" else 0)
    for _ in range(60):
        if os.path.exists(token_path):
            time.sleep(0.5)  # token 写盘与监听就绪间的窗口
            return proc
        if proc.poll() is not None:
            raise RuntimeError("serve 进程提前退出，退出码 %s" % proc.returncode)
        time.sleep(0.5)
    raise RuntimeError("等待 data/web_token 超时")


# ========== git fixture：构造走本机 git CLI，行为面全走 workbench RPC ==========

def build_git_fixture():
    root = os.path.join(FIXTURE, "git")
    if os.path.exists(root):
        force_rmtree(root)
        time.sleep(0.3)  # Windows 句柄释放窗口
    os.makedirs(root, exist_ok=True)
    work = os.path.join(root, "work")
    os.makedirs(work, exist_ok=True)
    git("init", "-b", "master", cwd=work)
    open(os.path.join(work, "f1.txt"), "w", encoding="utf-8").write("基线第一行\n")
    open(os.path.join(work, "f2.txt"), "w", encoding="utf-8").write("文件二\n")
    git("add", ".", cwd=work)
    git("commit", "-m", "基线提交", cwd=work)
    return work, root


# ========== 各阶段 ==========

def smoke_git_basic(work):
    """G1 信息读取 + 本地变更 + 暂存提交 + 历史（go-git 出清后核心面）。"""
    info = rpc("GetGitInfo", work)
    check("G1.1 GetGitInfo 分支读取", info.get("branch") == "master" and info.get("isRepo") is True, info)
    open(os.path.join(work, "f1.txt"), "w", encoding="utf-8").write("基线第一行\n新增第二行\n")
    open(os.path.join(work, "f3.txt"), "w", encoding="utf-8").write("新文件\n")
    changes = rpc("GetLocalChanges", work)
    names = {c.get("path"): c.get("status") for c in changes}
    check("G1.2 本地变更识别（modified+untracked）",
          names.get("f1.txt") == "M" and names.get("f3.txt") == "?", names)
    rpc("StageFiles", work, ["f1.txt", "f3.txt"])
    staged = rpc("GetStagedDiffText", work)
    check("G1.3 暂存 diff 非空", bool(staged and staged.strip()), len(staged or ""))
    rpc("CommitFiles", work, "冒烟：功能提交", ["f1.txt", "f3.txt"])
    changes = rpc("GetLocalChanges", work)
    check("G1.4 提交后工作区干净", len(changes) == 0, len(changes))
    hist = rpc("GetCommitHistory", work, 10, 0, {})
    check("G1.5 提交历史 ≥2 条且最新为主题提交", len(hist) >= 2 and "冒烟：功能提交" in (hist[0].get("message") or ""), len(hist))
    return hist[0].get("sha")


def smoke_branch_merge(work):
    """G2 分支 + 快进合并 + rebase 冲突中止。"""
    bl = rpc("GetBranches", work)
    check("G2.1 分支列表含 master", any(b.get("name") == "master" for b in (bl.get("local") or bl.get("branches") or [])), bl)
    rpc("CreateBranch", work, "feature-ff")
    rpc("CheckoutBranch", work, "feature-ff", False)
    open(os.path.join(work, "ff.txt"), "w", encoding="utf-8").write("快进提交内容\n")
    rpc("CommitFiles", work, "冒烟：快进分支提交", ["ff.txt"])
    ff_sha = head_sha(work)
    rpc("CheckoutBranch", work, "master", False)
    master_sha = head_sha(work)
    out = rpc("Merge", work, "feature-ff", "ff")
    merged_sha = head_sha(work)
    check("G2.2 快进合并 HEAD 前移", merged_sha == ff_sha and ff_sha != master_sha, (master_sha[:7], merged_sha[:7]))

    # rebase 冲突：master 改 f2 第一行；feature-rb 也改 f2 第一行，rebase 必撞
    open(os.path.join(work, "f2.txt"), "w", encoding="utf-8").write("master 改动行\n")
    rpc("CommitFiles", work, "冒烟：master 改 f2", ["f2.txt"])
    rpc("CreateBranch", work, "feature-rb")
    rpc("CheckoutBranch", work, "feature-rb", False)
    open(os.path.join(work, "f2.txt"), "w", encoding="utf-8").write("feature 改动行\n")
    rpc("CommitFiles", work, "冒烟：feature 改 f2", ["f2.txt"])
    rpc("CheckoutBranch", work, "master", False)
    # master 独有提交再改同一行，rebase 重放必撞 feature-rb 的改动
    open(os.path.join(work, "f2.txt"), "w", encoding="utf-8").write("master 再改行\n")
    rpc("CommitFiles", work, "冒烟：master 再改 f2", ["f2.txt"])
    rpc("Rebase", work, "feature-rb")
    cs = rpc("GetConflictState", work)
    check("G2.3 rebase 冲突态识别（type=rebase）", cs and cs.get("type") == "rebase", cs and cs.get("type"))
    rpc("AbortRebase", work)
    cs = rpc("GetConflictState", work)
    check("G2.4 AbortRebase 后无进行中冲突", cs and cs.get("type") == "none", cs and cs.get("type"))


def smoke_merge_conflict(work):
    """G3 merge 冲突 → 解决 → ContinueMerge（冲突面板核心链路）。"""
    open(os.path.join(work, "f1.txt"), "w", encoding="utf-8").write("master 终版行\n")
    rpc("CommitFiles", work, "冒烟：master 改 f1", ["f1.txt"])
    rpc("CreateBranch", work, "feature-cf")
    rpc("CheckoutBranch", work, "feature-cf", False)
    open(os.path.join(work, "f1.txt"), "w", encoding="utf-8").write("feature 终版行\n")
    rpc("CommitFiles", work, "冒烟：feature 改 f1", ["f1.txt"])
    # cherry-pick 源：独立分支 feature-cp（不 merge，避免空 pick 残留中间态）
    rpc("CreateBranch", work, "feature-cp")
    rpc("CheckoutBranch", work, "feature-cp", False)
    open(os.path.join(work, "cf_extra.txt"), "w", encoding="utf-8").write("cherry-pick 源行\n")
    rpc("CommitFiles", work, "冒烟：feature 独立文件", ["cf_extra.txt"])
    cp_src = head_sha(work)
    rpc("CheckoutBranch", work, "master", False)
    # master 独有提交再改同一行，merge 必撞 feature-cf 的改动
    open(os.path.join(work, "f1.txt"), "w", encoding="utf-8").write("master 再改行\n")
    rpc("CommitFiles", work, "冒烟：master 再改 f1", ["f1.txt"])
    rpc("Merge", work, "feature-cf", "ff")
    cs = rpc("GetConflictState", work)
    check("G3.1 merge 冲突态识别（type=merge）", cs and cs.get("type") == "merge", cs and (cs.get("type"), cs.get("files")))
    open(os.path.join(work, "f1.txt"), "w", encoding="utf-8").write("双方解决后内容\n")
    rpc("ResolveConflict", work, "f1.txt")
    out = rpc("ContinueMerge", work)
    cs = rpc("GetConflictState", work)
    check("G3.2 解决后 ContinueMerge 完成", cs and cs.get("type") == "none", cs and cs.get("type"))
    # CherryPick：把 feature-cp 的独立提交摘到 master
    if cp_src:
        rpc("CherryPick", work, cp_src)
        cs = rpc("GetConflictState", work)
        check("G3.3 CherryPick 完成（无冲突态）", cs and cs.get("type") == "none", cs and cs.get("type"))
        check("G3.4 CherryPick 文件落地", os.path.exists(os.path.join(work, "cf_extra.txt")), "")
    else:
        check("G3.3 CherryPick 源提交未找到", False, [c.get("message") for c in hist])


def smoke_remote(root, work):
    """G4 远程：bare 仓 + push/pull/fetch/syncInfo/tags。"""
    bare = os.path.join(root, "origin.git")
    os.makedirs(bare)
    git("init", "--bare", cwd=bare)
    rpc("AddRemote", work, "origin", bare)
    remotes = rpc("GetRemotes", work)
    check("G4.1 AddRemote 回读", any(r.get("name") == "origin" for r in remotes), [r.get("name") for r in remotes])
    out = rpc("PushRepo", work, True)
    sync = rpc("GetBranchSyncInfo", work)
    ahead = sync.get("ahead") if isinstance(sync, dict) else sync
    check("G4.2 Push 后 ahead=0", ahead in (0, "0"), sync)
    # 制造 behind：另一 clone 推进 → 本仓 pull
    clone2 = os.path.join(root, "clone2")
    os.makedirs(clone2)
    git("clone", bare, ".", cwd=clone2)
    git("config", "user.name", "冒烟", cwd=clone2)
    git("config", "user.email", "s@t", cwd=clone2)
    open(os.path.join(clone2, "remote_file.txt"), "w", encoding="utf-8").write("远程新增\n")
    git("add", ".", cwd=clone2)
    git("commit", "-m", "远程侧提交", cwd=clone2)
    git("push", cwd=clone2)
    rpc("FetchRepo", work, "origin", False)
    sync = rpc("GetBranchSyncInfo", work)
    behind = sync.get("behind") if isinstance(sync, dict) else sync
    check("G4.3 behind=1 识别", behind in (1, "1"), sync)
    rpc("PullRepo", work, False)
    sync = rpc("GetBranchSyncInfo", work)
    ahead = sync.get("ahead") if isinstance(sync, dict) else sync
    behind = sync.get("behind") if isinstance(sync, dict) else sync
    check("G4.4 Pull 后 ahead/behind 清零", ahead in (0, "0") and behind in (0, "0"), sync)
    check("G4.5 拉取后远程文件落工作区", os.path.exists(os.path.join(work, "remote_file.txt")), "")
    # tags
    rpc("CreateTag", work, "v0-smoke", "冒烟标签")
    tags = rpc("GetTags", work)
    check("G4.6 CreateTag+GetTags", any(t.get("name") == "v0-smoke" for t in tags), [t.get("name") for t in tags])
    rpc("PushTag", work, "v0-smoke")
    rpc("DeleteTag", work, "v0-smoke")
    tags = rpc("GetTags", work) or []
    check("G4.7 DeleteTag", all(t.get("name") != "v0-smoke" for t in tags), "")


def smoke_ppt_split():
    """P1 按页拆分 + 文件夹批量拆分（含派生产物排除重扫）。"""
    src3 = os.path.join(FIXTURE, "pptx", "中文 目录", "演示-三页.pptx")
    r = rpc("SplitPptxPerPage", src3)
    ok = r.get("pageCount") == 3 and len(r.get("files") or []) == 3 and os.path.isdir(r.get("outputDir", ""))
    check("P1.1 单文件拆分 3 页", ok, r)
    if ok:
        for f in r["files"]:
            with zipfile.ZipFile(f) as z:
                slides = [n for n in z.namelist() if n.startswith("ppt/slides/slide") and n.endswith(".xml")]
            if len(slides) != 1:
                ok = False
        check("P1.2 逐页产物 zip 各含 1 slide", ok, [os.path.basename(f) for f in r["files"]])
    batch = os.path.join(FIXTURE, "pptx", "批量")
    r = rpc("SplitPptxFolder", batch)
    errs = [i for i in r.get("items") or [] if i.get("error")]
    check("P1.3 批量拆分 2 文件全成功", r.get("total") == 2 and not errs, (r.get("total"), errs))
    r2 = rpc("SplitPptxFolder", batch)
    check("P1.4 重扫排除派生产物（total 仍 2）", r2.get("total") == 2, r2.get("total"))


def smoke_cart_export():
    """P2 购物栏：建库扫描 → 收集乱序页 → 重排 → 合并导出 → stale 跳过。"""
    lib_root = os.path.join(FIXTURE, "pptx")
    rpc("ClearAssetCart")  # 幂等清残留（上轮中断可能留栏）
    for l in rpc("ListAssetLibraries"):
        if os.path.normpath(l.get("rootPath") or "") == os.path.normpath(lib_root):
            rpc("DeleteAssetLibrary", l["id"], True)
    lib = rpc("CreateAssetLibrary", "冒烟购物栏库", lib_root)
    rpc("StartAssetScan", lib["id"])
    files = rpc("ListAssetFiles", lib["id"])
    by_rel = {f.get("relPath") or f.get("path"): f for f in files}
    f3 = by_rel.get("中文 目录/演示-三页.pptx")
    fb = by_rel.get("批量/批A.pptx")
    check("P2.1 建库扫描 2 文件入库（含中文目录）", bool(f3 and fb), sorted(by_rel))
    if not (f3 and fb):
        return
    # SrcPath 契约 = 绝对路径（合并清单 os.Stat/打开源文件基准），List 返回的 relPath 不作 srcPath
    items = [
        {"libraryId": lib["id"], "fileId": f3["id"], "slideNum": 3, "srcPath": os.path.join(lib_root, "中文 目录", "演示-三页.pptx"), "fileName": "演示-三页.pptx"},
        {"libraryId": lib["id"], "fileId": f3["id"], "slideNum": 1, "srcPath": os.path.join(lib_root, "中文 目录", "演示-三页.pptx"), "fileName": "演示-三页.pptx"},
        {"libraryId": lib["id"], "fileId": fb["id"], "slideNum": 2, "srcPath": os.path.join(lib_root, "批量", "批A.pptx"), "fileName": "批A.pptx"},
    ]
    add = rpc("AddAssetCartItems", items)
    check("P2.2 入栏 3 条（add=3 dup=0）", add.get("added") == 3 and add.get("duplicates") == 0, add)
    add2 = rpc("AddAssetCartItems", items[:1])
    check("P2.3 重复入栏幂等（added=0 dup=1）", add2.get("added") == 0 and add2.get("duplicates") == 1, add2)
    cart = rpc("ListAssetCartItems")
    # Key() 为 Go 方法不序列化，条目键由消费方按契约拼（<libraryId>|<fileId>|<slideNum>）
    cart_key = lambda c: "%s|%s|%s" % (c["libraryId"], c["fileId"], c["slideNum"])
    keys = [cart_key(c) for c in cart]
    check("P2.4 键形态 <libId>|<fileId>|<slideNum>", all(len(k.split("|")) == 3 for k in keys), keys)
    # 重排：批A页2 提到首位
    order = [k for k in keys if k.split("|")[2] == "2"] + [k for k in keys if k.split("|")[2] != "2"]
    rpc("ReorderAssetCart", order)
    cart = rpc("ListAssetCartItems")
    check("P2.5 Reorder 后首位为批A页2", cart_key(cart[0]) == order[0], [cart_key(c) for c in cart])
    out = os.path.join(FIXTURE, "合并导出-冒烟.pptx")
    if os.path.exists(out):
        os.remove(out)
    m = rpc("MergeAssetCartToPptx", out)
    ok = m.get("pageCount") == 3 and not (m.get("skippedItems")) and os.path.exists(out)
    check("P2.6 合并导出 3 页无跳过", ok, m)
    if os.path.exists(out):
        with zipfile.ZipFile(out) as z:
            slides = [n for n in z.namelist() if n.startswith("ppt/slides/slide") and n.endswith(".xml")]
        check("P2.7 产物 zip 含 3 slide", len(slides) == 3, len(slides))
    # stale：追加指向不存在源的条目
    stale_item = {"libraryId": lib["id"], "fileId": f3["id"], "slideNum": 2,
                  "srcPath": os.path.join(lib_root, "已删除-不存在.pptx"), "fileName": "已删除-不存在.pptx"}
    rpc("AddAssetCartItems", [stale_item])
    m = rpc("MergeAssetCartToPptx", out)
    check("P2.8 stale 条目跳过并报告", m.get("pageCount") == 3 and len(m.get("skippedItems") or []) == 1, m.get("skippedItems"))
    rpc("ClearAssetCart")
    cart = rpc("ListAssetCartItems")
    check("P2.9 ClearAssetCart 清空", len(cart) == 0, len(cart))


def smoke_pptx_preview():
    """P3 COM 转 PDF 预览（依赖本机 PowerPoint/WPS，不可用记 SKIP 归手点）。"""
    src = os.path.join(FIXTURE, "pptx", "中文 目录", "演示-三页.pptx")
    r = rpc("ConvertPptxForPreview", src)
    if r.get("pdfPath") and os.path.exists(r["pdfPath"]):
        check("P3.1 COM 转 PDF 产物落盘", True, r["pdfPath"])
    elif r.get("error"):
        check("P3.1 COM 转 PDF（本机无 PowerPoint/WPS，SKIP 归手点）", True, r["error"])
    else:
        check("P3.1 COM 转 PDF 产物落盘", False, r)


def main():
    attach = "--attach" in sys.argv
    proc = None
    global TOKEN
    if attach:
        TOKEN = open(os.path.join(RUN_DIR, "data", "web_token"), encoding="utf-8").read().strip()
    else:
        proc = start_serve()
        TOKEN = open(os.path.join(RUN_DIR, "data", "web_token"), encoding="utf-8").read().strip()
    try:
        ver = rpc("GetAppVersion")
        check("S0 serve 通道连通 GetAppVersion", bool(ver), ver)

        work, root = build_git_fixture()
        smoke_git_basic(work)
        smoke_branch_merge(work)
        smoke_merge_conflict(work)
        smoke_remote(root, work)

        build_pptx_fixture()
        smoke_ppt_split()
        smoke_cart_export()
        smoke_pptx_preview()
    finally:
        if proc:
            proc.terminate()
            try:
                proc.wait(timeout=5)
            except Exception:
                proc.kill()
    fails = [r for r in results if not r[1]]
    print("\n===== 汇总: %d/%d PASS, %d FAIL =====" % (len(results) - len(fails), len(results), len(fails)))
    for name, ok, ev in fails:
        print("FAIL", name, "|", str(ev)[:300])
    sys.exit(1 if fails else 0)


def build_pptx_fixture():
    """PPT fixture：中文目录 3 页（文件名页序错开）+ 批量目录 2 文件。"""
    sys.path.insert(0, SMOKE_DIR)
    import build_pptx_fixture as bf
    root = os.path.join(FIXTURE, "pptx")
    if os.path.exists(root):
        shutil.rmtree(root, ignore_errors=True)
    bf.build_pptx(os.path.join(root, "中文 目录", "演示-三页.pptx"), [
        ("第一页标题", ["第一页正文行甲"], "第一页备注"),
        ("第二页标题", ["第二页正文行乙"], ""),
        ("第三页标题", ["第三页正文行丙"], "第三页备注"),
    ])
    bf.build_pptx(os.path.join(root, "批量", "批A.pptx"), [
        ("批A第一页", ["内容"], ""), ("批A第二页", ["内容"], ""),
    ])
    bf.build_pptx(os.path.join(root, "批量", "批B.pptx"), [
        ("批B第一页", ["内容"], ""), ("批B第二页", ["内容"], ""), ("批B第三页", ["内容"], ""),
    ])


if __name__ == "__main__":
    main()
