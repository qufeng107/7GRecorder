#!/usr/bin/env python3
"""Archive the public, published OpenLive documentation; no account credentials needed."""

import argparse
import hashlib
import html
import json
from pathlib import Path
import re
import time
from datetime import datetime, timezone
from urllib.parse import urlsplit, urlunsplit
from urllib.request import Request, urlopen


INDEX_URL = "https://member.bilibili.com/arcopen/user/open-doc/view?id=13"
PAGE_BASE = "https://open-live.bilibili.com/document/"


def digest(data):
    return hashlib.sha256(data).hexdigest()


def public_url(value):
    parts = urlsplit(html.unescape(value))
    if not parts.netloc or not (
        parts.hostname == "member.bilibili.com"
        or parts.hostname == "hdslb.com"
        or (parts.hostname or "").endswith(".hdslb.com")
    ):
        raise ValueError("Unexpected document resource host")
    # CDN resources are publicly available without the index's signed query strings.
    query = parts.query if parts.hostname == "member.bilibili.com" else ""
    return urlunsplit(("https", parts.netloc, parts.path, query, ""))


def fetch(url, limit=25 * 1024 * 1024):
    for attempt in range(3):
        try:
            request = Request(url, headers={"User-Agent": "7GRecorder-DocumentationArchive/1.0"})
            with urlopen(request, timeout=35) as response:
                public_url(response.url)
                data = response.read(limit + 1)
                if len(data) > limit:
                    raise ValueError("Document resource exceeds size limit")
                return data, response.headers.get("Content-Type", "")
        except Exception:
            if attempt == 2:
                raise
            time.sleep(attempt + 1)


def articles(nodes, parents=()):
    for node in nodes:
        path = (*parents, node["name"])
        if node["type"] == "Article":
            yield node, path
        elif node["type"] == "Folder":
            yield from articles(node.get("children", []), path)
        else:
            raise ValueError("Unknown published directory node type")


def transform_prose(text, transform):
    """Leave fenced code examples byte-for-byte intact when rewriting links."""
    output, prose = [], []
    fence = None
    for line in text.splitlines(keepends=True):
        marker = re.match(r"^ {0,3}(`{3,}|~{3,})(.*)$", line.rstrip("\r\n"))
        if fence is None and marker:
            output.append(transform("".join(prose)))
            prose = []
            fence = marker.group(1)
            output.append(line)
        elif fence is not None:
            output.append(line)
            if marker and marker.group(1)[0] == fence[0] and len(marker.group(1)) >= len(fence) and not marker.group(2).strip():
                fence = None
        else:
            prose.append(line)
    output.append(transform("".join(prose)))
    return "".join(output)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", type=Path, required=True, help="New snapshot directory (must not exist)")
    args = parser.parse_args()
    root = args.output
    if root.exists():
        parser.error("Output already exists; use a new snapshot directory")
    payload, _ = fetch(INDEX_URL, 4 * 1024 * 1024)
    envelope = json.loads(payload)
    if envelope.get("code") != 0:
        raise ValueError("Public documentation index returned an error")
    index = envelope["data"]
    # Never archive draft_doc: only the tree published by the website is in scope.
    tree = json.loads(index["online_doc"])
    entries = list(articles(tree))
    if not entries or len({node["id"] for node, _ in entries}) != len(entries):
        raise ValueError("Published directory is empty or has duplicate article IDs")
    root.mkdir(parents=True)
    for directory in ["originals", "pages", "assets"]:
        (root / directory).mkdir()
    manifest = {
        "title": index["chapter"],
        "fetched_at_utc": datetime.now(timezone.utc).isoformat(),
        "index_url": INDEX_URL,
        "published_tree_sha256": digest(index["online_doc"].encode()),
        "upstream_index_mtime": index.get("mtime"),
        "method": "Public published directory and original Markdown; discovered with Playwright/Chrome",
        "complete": False,
        "articles": [],
        "assets": [],
        "asset_failures": [],
    }
    asset_paths = {}
    texts = {}
    link_map = {node["id"]: node["id"] + ".md" for node, _ in entries}
    # Match image URLs in Markdown and embedded HTML. PDF/SDK/EXE links are not fetched.
    image_pattern = re.compile(r"(?:https?:)?//[^\s<>\"')]+?\.(?:png|jpe?g|gif|webp|svg)(?:[?#][^\s<>\"')]*|)(?=[\s<>\"')]|$)", re.I)
    for number, (node, path) in enumerate(entries, 1):
        source = public_url(node["md_url"])
        data, content_type = fetch(source, 4 * 1024 * 1024)
        text = data.decode("utf-8-sig")
        if "text/html" in content_type or not text.strip():
            raise ValueError("Expected nonempty Markdown for " + node["id"])
        original = "originals/" + link_map[node["id"]]
        (root / original).write_bytes(data)
        texts[node["id"]] = text
        manifest["articles"].append({
            "id": node["id"], "title": node["name"], "navigation": list(path),
            "url": PAGE_BASE + node["id"], "markdown_url": source,
            "original_path": original, "original_sha256": digest(data),
            "bytes": len(data), "path": "pages/" + link_map[node["id"]],
        })
        prose_images = []
        def collect_images(prose):
            prose_images.extend(image_pattern.findall(prose))
            return prose
        transform_prose(text, collect_images)
        for image_url in prose_images:
            url = public_url(image_url)
            if url in asset_paths:
                continue
            asset_paths[url] = None
            try:
                asset, mime = fetch(url)
                if not mime.lower().startswith("image/"):
                    raise ValueError("Expected image content type")
                extension = Path(urlsplit(url).path).suffix.lower()
                target = "assets/" + digest(asset) + extension
                (root / target).write_bytes(asset)
                asset_paths[url] = target
                manifest["assets"].append({"url": url, "path": target, "sha256": digest(asset), "bytes": len(asset)})
            except Exception as exc:
                manifest["asset_failures"].append({"url": url, "error_type": type(exc).__name__})
        print(f"[{number}/{len(entries)}] {' / '.join(path)}", flush=True)
    for entry in manifest["articles"]:
        text = texts[entry["id"]]
        def image_link(match):
            value = match.group(0)
            target = asset_paths.get(public_url(value))
            return "../" + target if target else value
        text = transform_prose(text, lambda prose: image_pattern.sub(image_link, prose))
        def document_link(match):
            identity = match.group(1)
            return link_map.get(identity, match.group(0))
        text = transform_prose(text, lambda prose: re.sub(r"https?://open-live\.bilibili\.com/document/([a-f0-9-]{36})", document_link, prose))
        header = f"> 官方文档快照 · [在线原文]({entry['url']}) · 抓取时间：{manifest['fetched_at_utc']}\n\n"
        output = (header + text).encode()
        (root / entry["path"]).write_bytes(output)
        entry["sha256"] = digest(output)
    manifest["complete"] = not manifest["asset_failures"]
    (root / "manifest.json").write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n")
    lines = [
        "# " + index["chapter"] + "：官方文档快照", "",
        "抓取时间（UTC）：" + manifest["fetched_at_utc"], "",
        "正文和图片来自公开文档，无需登录或开发者密钥。只归档网站发布目录，不保存草稿。", "",
        "- `pages/`：便于本地阅读的 Markdown，站内文章与图片链接已本地化。",
        "- `originals/`：官网原始 Markdown 字节，未改写。",
        "- `assets/`：正文引用的公开绝对地址图片；SDK、安装包、外部 PDF 和演示视频保留在线链接。",
        "- 原文中的相对资源地址（如视频封面 `./img/introduce.png`）无公开源地址，未归档。",
        "- `manifest.json`：来源、目录、时间、SHA-256、完整性与图片失败记录。", "",
        "版权归原作者/哔哩哔哩；本快照不改变原始授权，也不是本项目的接口规格。",
        "文档可能过时或自相矛盾；接入前以实际权限、固定版本和脱敏响应验证。", "",
        f"共 {len(entries)} 篇文档，{len(manifest['assets'])} 个图片资源；图片失败 {len(manifest['asset_failures'])} 个。", "",
        "## 目录", "",
    ]
    for entry in manifest["articles"]:
        lines.append(f"- [{' / '.join(entry['navigation'])}]({entry['path']})")
    (root / "README.md").write_text("\n".join(lines) + "\n")
    if manifest["asset_failures"]:
        raise SystemExit("Markdown saved, but some images failed; see manifest.json")


if __name__ == "__main__":
    main()
