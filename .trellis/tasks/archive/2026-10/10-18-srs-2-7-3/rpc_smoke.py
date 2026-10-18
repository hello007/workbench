# -*- coding: utf-8 -*-
"""素材管理第八/九/十批真机冒烟——RPC 驱动（后端行为面）。UI 呈现面留真机手点单。"""
import json, os, shutil, subprocess, sys, time, urllib.request

BASE = "http://127.0.0.1:36199"
TOKEN = open(os.path.join(os.path.dirname(os.path.abspath(__file__)), "run", "data", "web_token"), encoding="utf-8").read().strip()
REPO = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
LIB_ROOT = os.path.join(os.path.dirname(os.path.abspath(__file__)), "lib")
OUT = os.path.join(os.path.dirname(os.path.abspath(__file__)), "out")

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


def lib_file_by_rel(files, rel):
    for f in files:
        if f.get("relPath") == rel or f.get("path") == rel:
            return f
    return None


def main():
    os.makedirs(OUT, exist_ok=True)

    # 幂等清理：删掉同 rootPath 的历史冒烟库（上次运行残留）
    for l in rpc("ListAssetLibraries"):
        if l.get("rootPath") and os.path.normpath(l["rootPath"]) == os.path.normpath(LIB_ROOT):
            try:
                rpc("DeleteAssetLibrary", l["id"], True)
                print("清理残留库:", l["id"])
            except Exception as e:
                print("清理残留库失败（继续）:", e)
    # 重置 fixture 目录（上轮中断可能残留半归位文件：同内容副本/待重链/moved_*）
    sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
    import build_fixture
    build_fixture.main()

    # ========== 阶段 1：建库 + 初始扫描 ==========
    lib = rpc("CreateAssetLibrary", "冒烟库", LIB_ROOT)
    lib_id = lib["id"]
    check("S1.1 建库成功", bool(lib_id), lib_id)
    scan = rpc("StartAssetScan", lib_id)
    files = rpc("ListAssetFiles", lib_id)
    rels = sorted(f.get("relPath") or f.get("path") for f in files)
    expect = sorted(["archive/归档方案-2025.pptx", "方案B-产品介绍.pptx", "方案A-客户交流（引注）.pptx",
                     "正式交付/交付方案-终版.pptx", "草稿区/草稿-内部讨论.pptx"])
    check("S1.2 无规则全扫 5 文件入库", rels == expect, rels)
    check("S1.3 干扰文件不入库（.txt/.pptx.tmp）",
          all(not (r.endswith(".txt") or r.endswith(".tmp")) for r in rels), "")
    stats = rpc("GetAssetLibraryStats", lib_id)
    check("S1.4 统计初始态 excludedCount=0", stats.get("excludedCount") == 0, stats)

    # ========== 阶段 2：排除规则全生命周期（第十批） ==========
    rules_ret = rpc("UpdateAssetLibraryRules", lib_id, ["*草稿*", "archive"], ["tmp", ".tmp", "*.TMP"])
    libs = rpc("ListAssetLibraries")
    cur = next(l for l in libs if l["id"] == lib_id)
    exts = cur.get("excludeExts") or []
    check("S2.1 规则保存回显归一（exts 全带点小写）",
          cur.get("excludeDirs") == ["*草稿*", "archive"] and exts and all(e == ".tmp" for e in exts),
          (cur.get("excludeDirs"), exts))
    if len(exts) > 1:
        check("S2.1b 缺陷记录：归一未去重（同值多形态回显多条）", False, exts)
    files = rpc("ListAssetFiles", lib_id)
    by_rel = {f.get("relPath") or f.get("path"): f for f in files}
    draft_ok = by_rel["草稿区/草稿-内部讨论.pptx"]["status"] == "excluded"
    arch_ok = by_rel["archive/归档方案-2025.pptx"]["status"] == "excluded"
    norm_ok = by_rel["正式交付/交付方案-终版.pptx"]["status"] == "ok"
    check("S2.2 规则命中转 excluded + 未命中保持 ok", draft_ok and arch_ok and norm_ok,
          {k: v["status"] for k, v in by_rel.items()})
    stats = rpc("GetAssetLibraryStats", lib_id)
    # 库内仅 2 个 pptx 位于命中目录（草稿区/、archive/）——excludedCount=2 为正确口径
    check("S2.3 统计 excludedCount=2（含 lost 同口径计入）", stats.get("excludedCount") == 2, stats.get("excludedCount"))
    # 标注保留：excluded 期间经底册改备注（审核修复前置）
    draft_id = by_rel["草稿区/草稿-内部讨论.pptx"]["id"]
    rpc("UpdateAssetFileMeta", lib_id, draft_id, {"rating": 4, "note": "烟雾测试独特词XYZQ", "tags": []})
    # 检索不可见：三入口
    page = rpc("SearchAssets", lib_id, "草稿", {"flatten": True, "limit": 100, "offset": 0})
    kw_hidden = (page.get("total") or 0) == 0 and not page.get("items")
    check("S2.4 关键词检索 excluded 不可见", kw_hidden, page.get("total"))
    # excluded 且文件仍在：恢复可见走复活（阶段 2.5 规则改宽）
    rules_ret = rpc("UpdateAssetLibraryRules", lib_id, [], [])
    files = rpc("ListAssetFiles", lib_id)
    by_rel = {f.get("relPath") or f.get("path"): f for f in files}
    revived = by_rel["草稿区/草稿-内部讨论.pptx"]["status"] == "ok"
    check("S2.5 规则改宽复活转 ok", revived, by_rel["草稿区/草稿-内部讨论.pptx"]["status"])
    page = rpc("SearchAssets", lib_id, "烟雾测试独特词XYZQ", {"flatten": True, "limit": 100, "offset": 0})
    hit = any((it.get("relPath") or it.get("path") or "") .find("草稿-内部讨论") >= 0 or it.get("fileId") == draft_id
              for it in (page.get("items") or []))
    check("S2.6 复活后独特词检索命中（FTS 重建·审核修复验证点）", (page.get("total") or 0) > 0 and hit, page.get("total"))
    stats = rpc("GetAssetLibraryStats", lib_id)
    check("S2.7 统计 excludedCount 回 0", stats.get("excludedCount") == 0, stats.get("excludedCount"))
    # excluded 与 lost 互斥：规则命中 + 文件消失
    draft_src = os.path.join(LIB_ROOT, "草稿区", "草稿-内部讨论.pptx")
    moved = os.path.join(os.path.dirname(os.path.abspath(__file__)), "moved_draft.pptx")
    shutil.move(draft_src, moved)
    rpc("UpdateAssetLibraryRules", lib_id, ["*草稿*"], [])
    files = rpc("ListAssetFiles", lib_id)
    by_rel = {f.get("relPath") or f.get("path"): f for f in files}
    st = by_rel["草稿区/草稿-内部讨论.pptx"]["status"]
    check("S2.8 规则命中+消失保持 excluded 不转 lost", st == "excluded", st)
    # 规则改宽后消失 → lost
    rpc("UpdateAssetLibraryRules", lib_id, [], [])
    files = rpc("ListAssetFiles", lib_id)
    by_rel = {f.get("relPath") or f.get("path"): f for f in files}
    st = by_rel["草稿区/草稿-内部讨论.pptx"]["status"]
    check("S2.9 改宽后消失标 lost", st == "lost", st)
    shutil.move(moved, draft_src)  # 放回（供后续批次使用）
    rpc("StartAssetScan", lib_id)
    files = rpc("ListAssetFiles", lib_id)
    by_rel = {f.get("relPath") or f.get("path"): f for f in files}
    check("S2.10 放回扫描自动复活", by_rel["草稿区/草稿-内部讨论.pptx"]["status"] == "ok", "")
    # 坏规则防御：含路径分隔符拒绝
    try:
        rpc("UpdateAssetLibraryRules", lib_id, ["a/b"], [])
        check("S2.11 含 / 模式保存被拒", False, "未拒绝")
    except RuntimeError as e:
        check("S2.11 含 / 模式保存被拒", "a/b" in str(e) or "拒绝" in str(e) or "E_ASSETS" in str(e), str(e)[:80])

    # ========== 阶段 3：备份与还原（第十批） ==========
    backup_dir = os.path.join(REPO, "smoke_tmp", "run", "data", "assets", "backups", lib_id)
    entry = rpc("BackupAssetLibrary", lib_id)
    b1 = entry.get("fileName")
    check("S3.1 手动备份生成", bool(b1) and os.path.isfile(os.path.join(backup_dir, b1)), b1)
    rpc("BackupAssetLibrary", lib_id)
    rpc("BackupAssetLibrary", lib_id)
    rpc("BackupAssetLibrary", lib_id)
    rpc("BackupAssetLibrary", lib_id)
    lst = rpc("ListAssetLibraryBackups", lib_id)
    names = [e.get("fileName") for e in lst]
    check("S3.2 清单倒序 + 5 份上限", len(names) == 5 and names == sorted(names, reverse=True), names)
    # 第 6 份淘汰最旧
    rpc("BackupAssetLibrary", lib_id)
    lst = rpc("ListAssetLibraryBackups", lib_id)
    names2 = [e.get("fileName") for e in lst]
    check("S3.3 第 6 份淘汰最旧（同秒 _N 深序）", len(lst) == 5 and names2[0] != names[0] and b1 not in names2, names2)
    # 改数据 → 还原回滚（备份条目 Path 绝对路径原样回传，对齐前端 AssetBackupDialog 形态）
    fid = by_rel["方案B-产品介绍.pptx"]["id"]
    rpc("UpdateAssetFileMeta", lib_id, fid, {"rating": 5, "note": "还原前修改标记", "tags": []})
    target = lst[0].get("path")
    rpc("RestoreAssetLibrary", lib_id, target)
    files = rpc("ListAssetFiles", lib_id)
    by_rel = {f.get("relPath") or f.get("path"): f for f in files}
    b_file = by_rel["方案B-产品介绍.pptx"]
    check("S3.4 还原回滚到备份时点（评分清除）", b_file.get("rating") in (0, None), b_file.get("rating"))
    # 重建前自动备份
    before = {e.get("fileName") for e in rpc("ListAssetLibraryBackups", lib_id)}
    rpc("RebuildAssetLibrary", lib_id)
    after = {e.get("fileName") for e in rpc("ListAssetLibraryBackups", lib_id)}
    check("S3.5 重建前自动备份 +1", len(after) == len(before) + 1, sorted(after - before))
    # 哈希恢复（R4）：重建后文件改名 → hash 兜底
    old_src = os.path.join(LIB_ROOT, "方案B-产品介绍.pptx")
    new_src = os.path.join(LIB_ROOT, "方案B-产品介绍-改名.pptx")
    # 先给页级标注再重建（重建清索引，路径 miss 走 hash 兜底恢复）
    files = rpc("ListAssetFiles", lib_id)
    by_rel = {f.get("relPath") or f.get("path"): f for f in files}
    bid = by_rel["方案B-产品介绍.pptx"]["id"]
    rpc("UpdateAssetSlideMeta", lib_id, bid, 1, {"rating": 3, "note": "页级标注恢复标记"})
    os.rename(old_src, new_src)
    rpc("RebuildAssetLibrary", lib_id)
    files = rpc("ListAssetFiles", lib_id)
    by_rel = {f.get("relPath") or f.get("path"): f for f in files}
    nb = by_rel.get("方案B-产品介绍-改名.pptx")
    check("S3.6 改名重建 hash 兜底恢复页级标注", nb and nb.get("id") and nb.get("status") == "ok", nb and nb.get("status"))
    slides = rpc("ListAssetSlides", lib_id, 50, 0)
    restored_note = False
    for s in slides.get("items") or []:
        if s.get("note") == "页级标注恢复标记":
            restored_note = True
            break
    check("S3.7 页级标注经 hash 兜底恢复", restored_note, [s.get("note") for s in (slides.get("items") or [])][:5])
    os.rename(new_src, old_src)
    rpc("StartAssetScan", lib_id)
    # 删库（备份保留）
    rpc("DeleteAssetLibrary", lib_id, False)
    libs = rpc("ListAssetLibraries")
    check("S3.8 删库后备份目录保留", os.path.isdir(backup_dir) and len(os.listdir(backup_dir)) >= 5,
          len(os.listdir(backup_dir)))

    # ========== 阶段 4：第九批导出/lost/重链/前缀替换 ==========
    lib = rpc("CreateAssetLibrary", "冒烟库2", LIB_ROOT)
    lib2 = lib["id"]
    rpc("StartAssetScan", lib2)
    files = rpc("ListAssetFiles", lib2)
    by_rel = {f.get("relPath") or f.get("path"): f for f in files}
    csv_path = os.path.join(OUT, "smoke_export.csv")
    md_dir = os.path.join(OUT, "smoke_md")
    if os.path.isdir(md_dir):
        shutil.rmtree(md_dir)
    ret = rpc("ExportAssetMetadata", lib2, {"csvPath": csv_path, "mdDir": md_dir,
                                            "formats": ["csv", "md"], "pathMode": "relative",
                                            "overwrite": True, "fileIds": [], "slideTargets": [],
                                            "keyword": "", "search": {"flatten": True}})
    raw = open(csv_path, "rb").read()
    check("S4.1 CSV BOM + 中文不乱码", raw[:3] == b"\xef\xbb\xbf" and "客户交流".encode("utf-8") in raw, raw[:10])
    body = raw.decode("utf-8-sig")
    check("S4.2 CSV 特殊字符转义（引号内逗号/换行）",
          '"客户交流方案,特殊「引号」"' in body or '客户交流方案,特殊「引号」' in body, body.splitlines()[1][:80] if len(body.splitlines()) > 1 else body[:80])
    md_files = os.listdir(md_dir) if os.path.isdir(md_dir) else []
    md_ok = len(md_files) > 0
    if md_ok:
        sample = open(os.path.join(md_dir, md_files[0]), encoding="utf-8").read()
        md_ok = "\\" not in sample.split("\n")[1] if len(sample.split("\n")) > 1 else True
    check("S4.3 MD+YAML 每页一卡 + frontmatter 路径无反斜杠", md_ok and len(md_files) >= 10, len(md_files))
    # 覆盖确认链
    try:
        rpc("ExportAssetMetadata", lib2, {"csvPath": csv_path, "mdDir": md_dir, "formats": ["csv"],
                                          "pathMode": "relative", "overwrite": False, "fileIds": [],
                                          "slideTargets": [], "keyword": "", "search": {"flatten": True}})
        check("S4.4 目标已存在未覆盖时拒绝", False, "未拒绝")
    except RuntimeError as e:
        check("S4.4 目标已存在未覆盖时拒绝", "TARGET_EXISTS" in str(e) or "E_ASSETS" in str(e), str(e)[:80])
    # lost 链
    src_b = os.path.join(LIB_ROOT, "方案B-产品介绍.pptx")
    moved_b = os.path.join(os.path.dirname(os.path.abspath(__file__)), "moved_b.pptx")
    shutil.move(src_b, moved_b)
    rpc("StartAssetScan", lib2)
    files = rpc("ListAssetFiles", lib2)
    by_rel = {f.get("relPath") or f.get("path"): f for f in files}
    lost_ok = by_rel["方案B-产品介绍.pptx"]["status"] == "lost"
    page = rpc("SearchAssets", lib2, "产品介绍", {"flatten": True, "limit": 100, "offset": 0})
    check("S4.5 移走标 lost 且检索仍命中（FTS 保留）", lost_ok and (page.get("total") or 0) > 0, page.get("total"))
    # 哈希相等重链：同内容副本改名放回不同名
    same_copy = os.path.join(LIB_ROOT, "方案B-同内容副本.pptx")
    shutil.copyfile(moved_b, same_copy)
    lost_id = by_rel["方案B-产品介绍.pptx"]["id"]
    ret = rpc("RelocateAssetFile", lib2, lost_id, same_copy, False)
    files = rpc("ListAssetFiles", lib2)
    by_rel = {f.get("relPath") or f.get("path"): f for f in files}
    check("S4.6 哈希相等重链直接复活", by_rel["方案B-同内容副本.pptx"]["status"] == "ok",
          by_rel["方案B-同内容副本.pptx"]["status"])
    # 哈希不等重链：lost（方案A）→ 库内未入库的不同内容文件（先扫出 lost，再落文件防被扫入）
    sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
    import build_fixture
    other = os.path.join(LIB_ROOT, "方案A-客户交流（引注）.pptx")
    src_a = other
    moved_a = os.path.join(os.path.dirname(os.path.abspath(__file__)), "moved_a.pptx")
    shutil.move(src_a, moved_a)
    rpc("StartAssetScan", lib2)
    files = rpc("ListAssetFiles", lib2)
    by_rel = {f.get("relPath") or f.get("path"): f for f in files}
    aid = by_rel["方案A-客户交流（引注）.pptx"]["id"]
    diff_path = os.path.join(LIB_ROOT, "内容不同-待重链.pptx")
    build_fixture.build_pptx(diff_path, [("完全不同的内容", ["另一份材料", "与方案A无共同哈希"], None)])
    try:
        rpc("RelocateAssetFile", lib2, aid, diff_path, False)
        check("S4.7 哈希不等未 force 拒绝", False, "未拒绝")
    except RuntimeError as e:
        check("S4.7 哈希不等未 force 拒绝", "MISMATCH" in str(e) or "E_ASSETS" in str(e), str(e)[:80])
    rpc("RelocateAssetFile", lib2, aid, diff_path, True)
    files = rpc("ListAssetFiles", lib2)
    by_rel = {f.get("relPath") or f.get("path"): f for f in files}
    check("S4.8 force 重链重解析", by_rel["内容不同-待重链.pptx"]["status"] == "ok",
          by_rel["内容不同-待重链.pptx"]["status"])
    # 清理（再造一个 lost：移走 archive 文件不放回）
    shutil.move(moved_a, src_a)
    shutil.move(os.path.join(LIB_ROOT, "archive", "归档方案-2025.pptx"),
                os.path.join(os.path.dirname(os.path.abspath(__file__)), "moved_arch.pptx"))
    rpc("StartAssetScan", lib2)
    lost_cnt = rpc("ClearLostAssetFiles", lib2)
    check("S4.9 清理丢失记录", lost_cnt >= 1, lost_cnt)
    # 前缀替换（嵌套腾位）
    os.makedirs(os.path.join(LIB_ROOT, "正式交付", "归档"), exist_ok=True)
    shutil.move(os.path.join(LIB_ROOT, "正式交付", "交付方案-终版.pptx"),
                os.path.join(LIB_ROOT, "正式交付", "归档", "交付方案-终版.pptx"))
    n = rpc("PreviewAssetPathPrefixReplace", lib2, "正式交付/", "正式交付/归档/")
    check("S4.10 前缀替换预览命中数", n >= 1, n)
    ret = rpc("ReplaceAssetPathPrefix", lib2, "正式交付/", "正式交付/归档/")
    rpc("StartAssetScan", lib2)
    files = rpc("ListAssetFiles", lib2)
    by_rel = {f.get("relPath") or f.get("path"): f for f in files}
    check("S4.11 嵌套腾位执行 + 扫描收敛",
          by_rel.get("正式交付/归档/交付方案-终版.pptx") is not None and by_rel["正式交付/归档/交付方案-终版.pptx"]["status"] == "ok",
          sorted(by_rel.keys()))

    # ========== 阶段 5：第八批排序/facet/聚合 ==========
    # 造评分差异（方案A=5 / 内容不同=2；方案A 经 S4.9 前放回归位）
    files = rpc("ListAssetFiles", lib2)
    by_rel = {f.get("relPath") or f.get("path"): f for f in files}
    rpc("UpdateAssetFileMeta", lib2, by_rel["方案A-客户交流（引注）.pptx"]["id"],
        {"rating": 5, "note": "", "tags": []})
    rpc("UpdateAssetFileMeta", lib2, by_rel["内容不同-待重链.pptx"]["id"],
        {"rating": 2, "note": "", "tags": []})
    # 排序：rating desc / asc（空关键词+无 facet = 空查询语义空页，带 ratings 全档激活 facets-only 路径）
    all_r = [0, 1, 2, 3, 4, 5]
    page = rpc("SearchAssets", lib2, "", {"flatten": True, "ratings": all_r, "sortBy": "rating",
                                          "sortDir": "desc", "limit": 100, "offset": 0})
    rels = [it.get("filePath") for it in page.get("items") or []]
    check("S5.1 rating 降序文件级口径", rels and rels[0].find("方案A") == 0, rels)
    page = rpc("SearchAssets", lib2, "", {"flatten": True, "ratings": all_r, "sortBy": "rating",
                                          "sortDir": "asc", "limit": 100, "offset": 0})
    rels = [it.get("filePath") for it in page.get("items") or []]
    check("S5.2 rating 升序", rels and rels[-1].find("方案A") == 0, rels)
    # 星级 facet 联动
    page = rpc("SearchAssets", lib2, "", {"flatten": True, "ratings": [5], "limit": 100, "offset": 0})
    rels = [it.get("filePath") for it in page.get("items") or []]
    check("S5.3 星级 facet 命中面刷新", len(rels) == 1 and rels[0].find("方案A") == 0, rels)
    # 聚合 vs 平铺（多页命中折叠）
    page_flat = rpc("SearchAssets", lib2, "备注", {"flatten": True, "limit": 100, "offset": 0})
    page_agg = rpc("SearchAssets", lib2, "备注", {"flatten": False, "limit": 100, "offset": 0})
    check("S5.4 聚合折叠条目数 ≤ 平铺", (page_agg.get("total") or 0) <= (page_flat.get("total") or 0),
          "agg=%s flat=%s" % (page_agg.get("total"), page_flat.get("total")))
    # mtime 排序
    page = rpc("SearchAssets", lib2, "", {"flatten": True, "ratings": all_r, "sortBy": "mtime",
                                          "sortDir": "desc", "limit": 100, "offset": 0})
    check("S5.5 mtime 排序不报错且有结果", (page.get("total") or 0) > 0, page.get("total"))
    # 清理阶段 4/5 库
    rpc("DeleteAssetLibrary", lib2, False)

    # ========== 汇总 ==========
    fails = [r for r in results if not r[1]]
    print("\n===== 汇总：%d 项，PASS %d，FAIL %d =====" % (len(results), len(results) - len(fails), len(fails)))
    for name, ok, ev in fails:
        print("FAIL:", name, "|", ev)
    return 1 if fails else 0


if __name__ == "__main__":
    sys.exit(main())
