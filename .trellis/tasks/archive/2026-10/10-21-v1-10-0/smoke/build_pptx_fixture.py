# -*- coding: utf-8 -*-
"""pptx fixture 构造（复刻 archive/2026-10/10-18-srs-2-7-3/build_fixture.py 的 build_pptx）。"""
import os, zipfile

XML_DECL = '<?xml version="1.0" encoding="UTF-8" standalone="yes"?>\r\n'
PKG_RELS_NS = "http://schemas.openxmlformats.org/package/2006/relationships"
OFFICE_REL_NS = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"


def slide_xml(title, body_lines, notes):
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
    slide_files = ["slide%d.xml" % (n - i) for i in range(n)]
    notes_files = ["notesSlide%d.xml" % (i + 1) for i, s in enumerate(slides) if s[2]]

    parts = []
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
