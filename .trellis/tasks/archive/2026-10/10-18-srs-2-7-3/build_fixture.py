# -*- coding: utf-8 -*-
"""冒烟 fixture 构造：程序化生成 pptx（复刻 service 测试 fixture 形态）+ 测试库目录结构。"""
import os, zipfile, shutil, sys

XML_DECL = '<?xml version="1.0" encoding="UTF-8" standalone="yes"?>\r\n'
PKG_RELS_NS = "http://schemas.openxmlformats.org/package/2006/relationships"
OFFICE_REL_NS = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"


def slide_xml(title, body_lines, notes):
    """assetsSlideXML 形态：ctrTitle 占位符 + body 段（段内双 run 拆分）+ 独立备注页内容。"""
    b = XML_DECL
    b += ('<p:sld xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:r="%s" '
          'xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main"><p:cSld><p:spTree>' % OFFICE_REL_NS)
    if title:
        b += ('<p:sp><p:nvSpPr><p:cNvPr id="2" name="标题 1"/><p:nvPr><p:ph type="ctrTitle"/></p:nvPr></p:nvSpPr>'
              '<p:txBody><a:bodyPr/><a:p><a:r><a:rPr lang="zh-CN"/><a:t>%s</a:t></a:r></a:p></p:txBody></p:sp>' % title)
    for line in body_lines:
        r = line
        mid = len(r) // 2
        b += ('<p:sp><p:nvSpPr><p:cNvPr id="3" name="内容占位符 2"/><p:nvPr><p:ph type="body"/></p:nvPr></p:nvSpPr>'
              '<p:txBody><a:bodyPr/><a:p><a:r><a:t>%s</a:t></a:r><a:r><a:t>%s</a:t></a:r></a:p></p:txBody></p:sp>'
              % (r[:mid], r[mid:]))
    b += '</p:spTree></p:cSld></p:sld>'
    return b


def notes_xml(notes):
    b = XML_DECL
    b += ('<p:notes xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:r="%s" '
          'xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main"><p:cSld><p:spTree>' % OFFICE_REL_NS)
    b += ('<p:sp><p:nvSpPr><p:cNvPr id="2" name="备注占位符 1"/><p:nvPr><p:ph type="body"/></p:nvPr></p:nvSpPr>'
          '<p:txBody><a:bodyPr/><a:p><a:r><a:t>%s</a:t></a:r></a:p></p:txBody></p:sp>' % notes)
    b += '</p:spTree></p:cSld></p:notes>'
    return b


def master_xml(layout_count=1):
    b = XML_DECL
    b += ('<p:sldMaster xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:r="%s" '
          'xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main"><p:sldLayoutIdLst>' % OFFICE_REL_NS)
    for i in range(layout_count):
        b += '<p:sldLayoutId id="%d" r:id="rId%d"/>' % (2147483648 + i, i + 1)
    b += '</p:sldLayoutIdLst></p:sldMaster>'
    return b


def content_types(slide_files, notes_files):
    b = XML_DECL
    b += '<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">'
    b += '<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>'
    b += '<Default Extension="xml" ContentType="application/xml"/>'
    b += '<Override PartName="/ppt/presentation.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.presentation.main+xml"/>'
    b += '<Override PartName="/ppt/slideMasters/slideMaster1.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slideMaster+xml"/>'
    b += '<Override PartName="/ppt/slideLayouts/slideLayout1.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slideLayout+xml"/>'
    b += '<Override PartName="/ppt/theme/theme1.xml" ContentType="application/vnd.openxmlformats-officedocument.theme+xml"/>'
    for f in slide_files:
        b += '<Override PartName="/ppt/slides/%s" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slide+xml"/>' % f
    for n in notes_files:
        b += '<Override PartName="/ppt/notesSlides/%s" ContentType="application/vnd.openxmlformats-officedocument.presentationml.notesSlide+xml"/>' % n
    b += '</Types>'
    return b


def build_pptx(path, slides):
    """slides: [(title, [body...], notes)]；slide 文件名与页序刻意错开（三跳映射真实解析）。"""
    n = len(slides)
    slide_files = ["slide%d.xml" % (n - i) for i in range(n)]  # 页 i -> slideN-i.xml
    notes_files = ["notesSlide%d.xml" % (i + 1) for i, s in enumerate(slides) if s[2]]

    parts = []  # (part_name, content)
    parts.append(("[Content_Types].xml", content_types(slide_files, notes_files)))
    parts.append(("_rels/.rels", XML_DECL +
        '<Relationships xmlns="%s"><Relationship Id="rId1" Type="%s/officeDocument" Target="ppt/presentation.xml"/></Relationships>'
        % (PKG_RELS_NS, OFFICE_REL_NS)))
    pres = XML_DECL
    pres += ('<p:presentation xmlns:r="%s" xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main">' % OFFICE_REL_NS)
    pres += '<p:sldMasterIdLst><p:sldMasterId id="2147483648" r:id="rId1"/></p:sldMasterIdLst><p:sldIdLst>'
    for i in range(n):
        pres += '<p:sldId id="%d" r:id="rId%d"/>' % (256 + i, i + 2)
    pres += '</p:sldIdLst><p:sldSz cx="12192000" cy="6858000"/></p:presentation>'
    parts.append(("ppt/presentation.xml", pres))

    rels = XML_DECL
    rels += '<Relationships xmlns="%s">' % PKG_RELS_NS
    rels += '<Relationship Id="rId1" Type="%s/slideMaster" Target="slideMasters/slideMaster1.xml"/>' % OFFICE_REL_NS
    for i, f in enumerate(slide_files):
        rels += '<Relationship Id="rId%d" Type="%s/slide" Target="slides/%s"/>' % (i + 2, OFFICE_REL_NS, f)
    rels += '</Relationships>'
    parts.append(("ppt/_rels/presentation.xml.rels", rels))

    parts.append(("ppt/slideMasters/slideMaster1.xml", master_xml(1)))
    parts.append(("ppt/slideMasters/_rels/slideMaster1.xml.rels", XML_DECL +
        '<Relationships xmlns="%s"><Relationship Id="rId1" Type="%s/slideLayout" Target="../slideLayouts/slideLayout1.xml"/>'
        '<Relationship Id="rId2" Type="%s/theme" Target="../theme/theme1.xml"/></Relationships>'
        % (PKG_RELS_NS, OFFICE_REL_NS, OFFICE_REL_NS)))
    parts.append(("ppt/slideLayouts/slideLayout1.xml", XML_DECL +
        '<p:sldLayout xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:r="%s" '
        'xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main"/>' % OFFICE_REL_NS))
    parts.append(("ppt/slideLayouts/_rels/slideLayout1.xml.rels", XML_DECL +
        '<Relationships xmlns="%s"><Relationship Id="rId1" Type="%s/slideMaster" Target="../slideMasters/slideMaster1.xml"/></Relationships>'
        % (PKG_RELS_NS, OFFICE_REL_NS)))
    parts.append(("ppt/theme/theme1.xml", XML_DECL +
        '<a:theme xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" name="主题 1"/>'))

    for i, (title, body, notes) in enumerate(slides):
        f = slide_files[i]
        parts.append(("ppt/slides/" + f, slide_xml(title, body, notes)))
        sr = XML_DECL
        sr += '<Relationships xmlns="%s">' % PKG_RELS_NS
        sr += '<Relationship Id="rId1" Type="%s/slideLayout" Target="../slideLayouts/slideLayout1.xml"/>' % OFFICE_REL_NS
        if notes:
            sr += '<Relationship Id="rId2" Type="%s/notesSlide" Target="../notesSlides/notesSlide%d.xml"/>' % (OFFICE_REL_NS, i + 1)
        sr += '</Relationships>'
        parts.append(("ppt/slides/_rels/" + f + ".rels", sr))
        if notes:
            parts.append(("ppt/notesSlides/notesSlide%d.xml" % (i + 1), notes_xml(notes)))

    os.makedirs(os.path.dirname(path), exist_ok=True)
    with zipfile.ZipFile(path, "w", zipfile.ZIP_DEFLATED) as z:
        for name, content in parts:
            z.writestr(name, content.encode("utf-8"))


def main():
    root = os.path.join(os.path.dirname(os.path.abspath(__file__)), "lib")
    if os.path.exists(root):
        shutil.rmtree(root)
    os.makedirs(root)

    # 方案A：3 页，标题/备注含 CSV 特殊字符（逗号/引号/换行）——第九批导出面（文件名用全角引号，Windows 禁半角 "）
    build_pptx(os.path.join(root, '方案A-客户交流（引注）.pptx'), [
        ("客户交流方案,特殊「引号」", ["第一页要点：银行渠道\n数字化转型", "售前团队 2026"], "备注：本页讲背景，注意「引号」与,逗号"),
        ("产品能力矩阵", ["双粒度检索：文件级与单页级", "FTS5 中文分词"], "备注：第二页讲能力，含\"半角引号\"内容"),
        ("实施路线", ["三期推进：索引、标注、导出", "风险与回滚"], None),
    ])
    # 方案B：2 页（聚合展开/排序面）
    build_pptx(os.path.join(root, "方案B-产品介绍.pptx"), [
        ("产品介绍", ["整体架构与部署形态", "本地索引不上云"], "备注：开场页"),
        ("检索能力", ["关键词命中单页直达", "标签继承合并"], None),
    ])
    # 草稿区/：第十批 ExcludeDirs「*草稿*」命中
    build_pptx(os.path.join(root, "草稿区", "草稿-内部讨论.pptx"), [
        ("内部草稿", ["未定稿内容勿外发", "占位文本"], "备注：草稿页"),
        ("草稿附录", ["补充材料", "待删除"], None),
    ])
    # archive/：第十批 ExcludeDirs「archive」命中
    build_pptx(os.path.join(root, "archive", "归档方案-2025.pptx"), [
        ("2025 归档方案", ["历史版本留存", "不再维护"], None),
    ])
    # 正常子目录（不被排除，验证排除只命中规则目录）
    build_pptx(os.path.join(root, "正式交付", "交付方案-终版.pptx"), [
        ("交付终版", ["验收标准清单", "交付物列表"], "备注：终版说明"),
    ])
    # 后缀防御面：非 pptx 干扰文件（不应入库）
    with open(os.path.join(root, "说明.txt"), "w", encoding="utf-8") as f:
        f.write("非素材文件")
    with open(os.path.join(root, "伪装.pptx.tmp"), "w", encoding="utf-8") as f:
        f.write("tmp 干扰")

    print("fixture root:", root)
    for dirpath, dirnames, filenames in os.walk(root):
        for fn in filenames:
            print(" ", os.path.relpath(os.path.join(dirpath, fn), root))


if __name__ == "__main__":
    main()
