#!/usr/bin/env python3
"""Check the explainer bundle before it is rendered.

Stage 2 of the explainer: every rule here is one the bundle must satisfy before
render.py is allowed to turn it into a page. Standard library only, plus PyYAML
when it is installed (the bundle is YAML frontmatter, so a vendored parser is
the fallback).

Usage: python3 docs/explainer/check.py [bundle-dir]
Exit status is 1 when any check fails.
"""

from __future__ import annotations

import os
import re
import subprocess
import sys
from collections import Counter, defaultdict
from pathlib import Path

try:
    import yaml  # type: ignore
except ImportError:  # pragma: no cover - the fallback parser below is used
    yaml = None

HERE = Path(__file__).resolve().parent
REPO = HERE.parent.parent
BUNDLE = HERE / "bundle"

# A resource path may point into a sibling repo. This is the one convention the
# instructions do not define: a path under `../anam-importers/` is resolved in
# that checkout, and must be tracked there. The checkout sits beside this one unless
# EXPLAINER_IMPORTERS_DIR names it, as it must from a worktree under another folder.
SIBLINGS = {"../anam-importers/": Path(os.environ.get("EXPLAINER_IMPORTERS_DIR") or REPO.parent / "anam-importers")}

TYPES = {
    "purpose": {"commit", "primary_workflow"},
    "term": set(),
    "decision": {"alternatives", "affects"},
    "invariant": {"applies_to", "enforced_at"},
    "component": {"depends_on"},
    "workflow": {"trigger", "steps"},
    "data-structure": {"references"},
    "change-path": {"touches"},
    "gap": {"looked_at"},
}
ALWAYS = {"type", "title", "description", "timestamp"}
SHARED = {"resource", "basis", "tier"}  # required except on purpose, term, gap
NO_SHARED = {"purpose", "term", "gap"}
BASIS = {"code", "docs", "history", "local", "inferred"}
TIER = {"core", "supporting", "detail"}
CARD = {"1:1", "1:n", "n:1", "n:n"}
MAX_CORE = 9
MAX_LABEL = 40
MAX_UNANCHORED_CITATIONS = 3

LINK_RE = re.compile(r"\[[^\]]*\]\((/[^)\s]+\.md)(?:#[^)]*)?\)")


# --------------------------------------------------------------------------- parsing


def split_frontmatter(text: str, path: Path) -> tuple[dict, str]:
    if not text.startswith("---\n"):
        raise ValueError(f"{path}: does not start with a --- frontmatter fence")
    end = text.find("\n---\n", 4)
    if end < 0:
        raise ValueError(f"{path}: unterminated frontmatter fence")
    raw = text[4:end]
    body = text[end + 5 :]
    if yaml is not None:
        try:
            data = yaml.safe_load(raw) or {}
        except yaml.YAMLError as e:
            raise ValueError(f"{path}: frontmatter does not parse: {e}") from e
    else:
        data = _mini_yaml(raw)
    if not isinstance(data, dict):
        raise ValueError(f"{path}: frontmatter is not a mapping")
    return data, body


def _mini_yaml(raw: str) -> dict:
    """A deliberately small YAML subset: scalars, flow lists, block lists of
    scalars or of flat mappings. Enough for this bundle when PyYAML is absent."""
    out: dict = {}
    key = None
    items: list | None = None
    cur: dict | None = None
    for line in raw.splitlines():
        if not line.strip() or line.lstrip().startswith("#"):
            continue
        indent = len(line) - len(line.lstrip())
        s = line.strip()
        if indent == 0:
            if items is not None and key is not None:
                out[key] = items
            items, cur = None, None
            k, _, v = s.partition(":")
            key = k.strip()
            v = v.strip()
            if v == "":
                items = []
            else:
                out[key] = _scalar(v)
        elif s.startswith("- "):
            if cur is not None:
                items.append(cur)  # type: ignore[union-attr]
                cur = None
            rest = s[2:].strip()
            if ":" in rest and not rest.startswith(("'", '"', "/")):
                cur = {}
                k, _, v = rest.partition(":")
                cur[k.strip()] = _scalar(v.strip())
            else:
                items.append(_scalar(rest))  # type: ignore[union-attr]
        else:
            k, _, v = s.partition(":")
            if cur is None:
                cur = {}
            cur[k.strip()] = _scalar(v.strip())
    if cur is not None and items is not None:
        items.append(cur)
    if items is not None and key is not None:
        out[key] = items
    return out


def _scalar(v: str):
    if v.startswith("[") and v.endswith("]"):
        inner = v[1:-1].strip()
        return [] if not inner else [_scalar(x.strip()) for x in inner.split(",")]
    if len(v) >= 2 and v[0] == v[-1] and v[0] in "'\"":
        body = v[1:-1]
        # YAML's two quoted styles: a double-quoted scalar carries escapes, and a
        # single-quoted one writes a quote by doubling it.
        return _unescape(body) if v[0] == '"' else body.replace("''", "'")
    if v in ("true", "True"):
        return True
    if v in ("false", "False"):
        return False
    if re.fullmatch(r"-?\d+", v):
        return int(v)
    return v


# The escapes a double-quoted YAML scalar defines. PyYAML reads them; the fallback
# must too, or an anchor written as YAML reads as something else.
_ESCAPES = {
    '"': '"', "\\": "\\", "/": "/", " ": " ", "a": "\a", "b": "\b", "t": "\t",
    "\t": "\t", "n": "\n", "v": "\v", "f": "\f", "r": "\r", "e": "\x1b", "0": "\0",
}


def _unescape(s: str) -> str:
    """Unescape a double-quoted YAML scalar, as PyYAML does."""
    out: list[str] = []
    i = 0
    while i < len(s):
        c = s[i]
        if c != "\\" or i + 1 >= len(s):
            out.append(c)
            i += 1
            continue
        n = s[i + 1]
        if n in _ESCAPES:
            out.append(_ESCAPES[n])
            i += 2
            continue
        # An escape YAML does not define is an error there; here it stays as it is.
        out.append(c)
        i += 1
    return "".join(out)


def load_bundle(bundle: Path) -> dict[str, dict]:
    concepts: dict[str, dict] = {}
    for path in sorted(bundle.rglob("*.md")):
        if path.name in ("index.md", "log.md"):
            continue
        rel = "/" + path.relative_to(bundle).as_posix()
        data, body = split_frontmatter(path.read_text(encoding="utf-8"), path)
        data["_id"] = rel
        data["_path"] = path
        data["_body"] = body
        concepts[rel] = data
    return concepts


# --------------------------------------------------------------------------- git and files

_tracked: dict[Path, set[str]] = {}


def tracked_files(root: Path) -> set[str]:
    if root not in _tracked:
        out = subprocess.run(
            ["git", "-C", str(root), "ls-files"], capture_output=True, text=True, check=True
        ).stdout
        _tracked[root] = set(out.splitlines())
    return _tracked[root]


def resolve_path(p: str) -> tuple[Path, Path, str] | None:
    """Return (repo root, absolute file, repo-relative path) for a resource path,
    or None when the path is not a tracked file."""
    for prefix, root in SIBLINGS.items():
        if p.startswith(prefix):
            rel = p[len(prefix) :]
            if rel in tracked_files(root):
                return root, root / rel, rel
            return None
    if p in tracked_files(REPO):
        return REPO, REPO / p, p
    return None


_text_cache: dict[Path, str] = {}


def file_text(path: Path) -> str:
    if path not in _text_cache:
        _text_cache[path] = path.read_text(encoding="utf-8", errors="replace")
    return _text_cache[path]


def symbol_defined(path: Path, symbol: str) -> bool:
    text = file_text(path)
    # `Type.method` names the method; the definition line carries the last segment.
    sym = re.escape(symbol.split(".")[-1])
    ext = path.suffix
    if ext == ".go":
        pats = [
            rf"CREATE (TABLE|TRIGGER|INDEX|VIEW)( IF NOT EXISTS)? {sym}\b",  # schema in a Go string

            rf"^func (\([^)]*\) )?{sym}\b",
            rf"^type {sym}\b",
            rf"^(var|const) {sym}\b",
            rf"^\t{sym}\s*(=|[A-Za-z\[\]*]+\s*=|$|\s+[A-Za-z\[\]*.]+\s*$)",  # inside var/const/type blocks
            rf"^\s+{sym}\s+[A-Za-z\[\]*.]+.*$",  # a struct field or const in a block
        ]
    elif ext == ".py":
        pats = [rf"^\s*def {sym}\b", rf"^\s*class {sym}\b", rf"^{sym}\s*(:|=)"]
    elif ext == ".sql":
        pats = [rf"CREATE (TABLE|TRIGGER|INDEX|VIEW)( IF NOT EXISTS)? {sym}\b"]
    elif ext in (".json",):
        pats = [rf'"{sym}"\s*:']
    elif ext in (".yaml", ".yml"):
        pats = [rf"^\s*{sym}\s*:", rf"^\s*-\s*{sym}\s*:"]
    elif ext == ".md":
        pats = [rf"^#+\s.*{sym}", rf"^\*\*{sym}\*\*", rf"^\| \*\*{sym}\*\*"]
    elif ext == ".sh":
        pats = [rf"^{sym}\s*\(\)", rf"^{sym}=", rf"^\w+=\"\$\(.*{sym}"]
    else:
        pats = [rf"^{sym}\b", rf"^\s*{sym}\s*[:=]"]
    return any(re.search(p, text, re.MULTILINE) for p in pats)


# --------------------------------------------------------------------------- checks


class Checker:
    def __init__(self, concepts: dict[str, dict]):
        self.c = concepts
        self.errors: list[str] = []
        self.citations: list[tuple[str, str, str | None]] = []  # (concept, resource, anchor)

    def err(self, cid: str, msg: str) -> None:
        self.errors.append(f"{cid}: {msg}")

    # -- helpers
    def is_type(self, link, t: str) -> bool:
        return isinstance(link, str) and link in self.c and self.c[link].get("type") == t

    def check_link(self, cid: str, link, what: str) -> None:
        if not isinstance(link, str) or not link.startswith("/") or not link.endswith(".md"):
            self.err(cid, f"{what}: {link!r} is not a /-rooted .md link")
        elif link not in self.c:
            self.err(cid, f"{what}: link {link} does not resolve to a concept")

    def check_links_list(self, cid: str, value, what: str) -> None:
        if value is None:
            self.err(cid, f"{what} is missing")
            return
        if not isinstance(value, list):
            self.err(cid, f"{what} must be a list")
            return
        for v in value:
            self.check_link(cid, v, what)

    def check_resource(self, cid: str, resource, anchor, where: str) -> None:
        if not isinstance(resource, str) or not resource:
            self.err(cid, f"{where}: resource is missing or not a string")
            return
        path, _, symbol = resource.partition("#")
        res = resolve_path(path)
        if res is None:
            self.err(cid, f"{where}: resource path {path!r} is not a tracked file")
            return
        _, abs_path, _ = res
        if symbol and not symbol_defined(abs_path, symbol):
            self.err(cid, f"{where}: symbol {symbol!r} is not defined in {path}")
        if anchor is not None:
            if not isinstance(anchor, str) or not anchor:
                self.err(cid, f"{where}: anchor must be a non-empty string")
            elif anchor not in file_text(abs_path):
                self.err(cid, f"{where}: anchor {anchor!r} does not occur in {path}")
        self.citations.append((cid, resource, anchor if isinstance(anchor, str) and anchor else None))

    def check_tested_by(self, cid: str, value) -> None:
        if value is None:
            return
        if not isinstance(value, list):
            self.err(cid, "tested_by must be a list")
            return
        for entry in value:
            if not isinstance(entry, str) or "#" not in entry:
                self.err(cid, f"tested_by entry {entry!r} is not path#TestName")
                continue
            path, _, name = entry.partition("#")
            res = resolve_path(path)
            if res is None:
                self.err(cid, f"tested_by path {path!r} is not a tracked file")
                continue
            text = file_text(res[1])
            if not re.search(rf"^\s*(func|def) {re.escape(name)}\b", text, re.MULTILINE) and name not in text:
                self.err(cid, f"tested_by {name!r} does not occur in {path}")

    # -- per concept
    def check_concept(self, cid: str, d: dict) -> None:
        t = d.get("type")
        if t not in TYPES:
            self.err(cid, f"type {t!r} is not one of {sorted(TYPES)}")
            return
        expected_dir = "/purpose/" if t == "purpose" else f"/{t}s/"
        if not cid.startswith(expected_dir):
            self.err(cid, f"a {t} concept must live under {expected_dir}")
        for k in ALWAYS | TYPES[t]:
            if k not in d or d[k] in (None, ""):
                self.err(cid, f"required key {k!r} is missing")
        if t not in NO_SHARED:
            for k in SHARED:
                if k not in d or d[k] in (None, ""):
                    self.err(cid, f"required key {k!r} is missing")
            if d.get("basis") not in BASIS:
                self.err(cid, f"basis {d.get('basis')!r} not in {sorted(BASIS)}")
            if d.get("tier") not in TIER:
                self.err(cid, f"tier {d.get('tier')!r} not in {sorted(TIER)}")
            self.check_resource(cid, d.get("resource"), d.get("anchor"), "frontmatter")
        elif "tier" in d and d["tier"] not in TIER:
            self.err(cid, f"tier {d.get('tier')!r} not in {sorted(TIER)}")
        if "basis" in d and d["basis"] not in BASIS:
            self.err(cid, f"basis {d.get('basis')!r} not in {sorted(BASIS)}")

        # body links
        for m in LINK_RE.finditer(d["_body"]):
            self.check_link(cid, m.group(1), "body link")

        if t == "purpose":
            self.check_link(cid, d.get("primary_workflow"), "primary_workflow")
            if not self.is_type(d.get("primary_workflow"), "workflow"):
                self.err(cid, "primary_workflow is not a workflow")
            if not isinstance(d.get("commit"), str) or not re.fullmatch(r"[0-9a-f]{7,40}", d.get("commit", "")):
                self.err(cid, "commit must be a git hash")
        elif t == "decision":
            if not isinstance(d.get("alternatives"), list) or not d["alternatives"]:
                self.err(cid, "alternatives must be a non-empty list")
            self.check_links_list(cid, d.get("affects"), "affects")
            self.check_tested_by(cid, d.get("tested_by"))
        elif t == "invariant":
            self.check_links_list(cid, d.get("applies_to"), "applies_to")
            ea = d.get("enforced_at")
            if ea != "not enforced":
                if not isinstance(ea, str) or "#" not in ea:
                    self.err(cid, "enforced_at must be path#symbol or 'not enforced'")
                else:
                    self.check_resource(cid, ea, d.get("enforced_anchor"), "enforced_at")
            self.check_tested_by(cid, d.get("tested_by"))
        elif t == "component":
            self.check_links_list(cid, d.get("depends_on"), "depends_on")
            for v in d.get("depends_on") or []:
                if isinstance(v, str) and v in self.c and not self.is_type(v, "component"):
                    self.err(cid, f"depends_on {v} is not a component")
        elif t == "workflow":
            self.check_workflow(cid, d)
        elif t == "data-structure":
            refs = d.get("references")
            if refs is None or not isinstance(refs, list):
                self.err(cid, "references must be a list (may be empty)")
            else:
                for r in refs:
                    if not isinstance(r, dict):
                        self.err(cid, f"reference {r!r} is not a mapping")
                        continue
                    self.check_link(cid, r.get("to"), "references.to")
                    if not self.is_type(r.get("to"), "data-structure"):
                        self.err(cid, f"references.to {r.get('to')} is not a data structure")
                    if not isinstance(r.get("how"), str) or not r["how"]:
                        self.err(cid, "references.how is missing")
                    if r.get("card") not in CARD:
                        self.err(cid, f"references.card {r.get('card')!r} not in {sorted(CARD)}")
        elif t == "change-path":
            self.check_links_list(cid, d.get("touches"), "touches")
        elif t == "gap":
            if not isinstance(d.get("looked_at"), (str, list)) or not d.get("looked_at"):
                self.err(cid, "looked_at is missing")

    def check_workflow(self, cid: str, d: dict) -> None:
        steps = d.get("steps")
        if not isinstance(steps, list) or not steps:
            self.err(cid, "steps must be a non-empty list")
            return
        for i, s in enumerate(steps, 1):
            where = f"step {i}"
            if not isinstance(s, dict):
                self.err(cid, f"{where}: not a mapping")
                continue
            for k in ("actor", "label", "action", "resource"):
                if k not in s or s[k] in (None, ""):
                    self.err(cid, f"{where}: required key {k!r} is missing")
            self.check_link(cid, s.get("actor"), f"{where} actor")
            if not self.is_type(s.get("actor"), "component"):
                self.err(cid, f"{where}: actor {s.get('actor')} is not a component")
            caller = s.get("caller")
            if caller is not None and caller != "outside":
                self.check_link(cid, caller, f"{where} caller")
                if not self.is_type(caller, "component"):
                    self.err(cid, f"{where}: caller {caller!r} is not a component, 'outside' or absent")
            label = s.get("label")
            if isinstance(label, str) and len(label) > MAX_LABEL:
                self.err(cid, f"{where}: label is {len(label)} characters, over {MAX_LABEL}: {label!r}")
            for k in ("reads", "writes"):
                v = s.get(k, [])
                if v is None:
                    v = []
                if not isinstance(v, list):
                    self.err(cid, f"{where}: {k} must be a list")
                    continue
                for link in v:
                    self.check_link(cid, link, f"{where} {k}")
                    if not self.is_type(link, "data-structure"):
                        self.err(cid, f"{where}: {k} {link} is not a data structure")
            if "ends" in s and not isinstance(s["ends"], bool):
                self.err(cid, f"{where}: ends must be true or false")
            if "branch" in s and (not isinstance(s["branch"], str) or not s["branch"]):
                self.err(cid, f"{where}: branch must be a non-empty string")
            self.check_resource(cid, s.get("resource"), s.get("anchor"), where)

    # -- whole bundle
    def check_bundle(self) -> None:
        by_type: dict[str, list[str]] = defaultdict(list)
        for cid, d in self.c.items():
            by_type[d.get("type", "?")].append(cid)
        if len(by_type["purpose"]) != 1:
            self.errors.append(f"bundle: expected exactly one purpose, found {len(by_type['purpose'])}")
        core = [cid for cid, d in self.c.items() if d.get("tier") == "core"]
        if len(core) > MAX_CORE:
            self.errors.append(f"bundle: {len(core)} core concepts, over {MAX_CORE}: {core}")

        touched: set[str] = set()
        in_workflow: set[str] = set()
        for cid in by_type["workflow"]:
            for s in self.c[cid].get("steps") or []:
                if not isinstance(s, dict):
                    continue
                for k in ("reads", "writes"):
                    for link in s.get(k) or []:
                        touched.add(link)
                in_workflow.add(s.get("actor"))
                if s.get("caller") not in (None, "outside"):
                    in_workflow.add(s.get("caller"))
        for cid in by_type["data-structure"]:
            if cid not in touched:
                self.err(cid, "data structure is read or written by no workflow step")
        depended: set[str] = set()
        for cid in by_type["component"]:
            for v in self.c[cid].get("depends_on") or []:
                depended.add(v)
        for cid in by_type["component"]:
            if cid not in in_workflow and cid not in depended:
                self.err(cid, "component appears in no workflow and nothing depends on it")

        counts = Counter(r for _, r, _ in self.citations)
        for cid, resource, anchor in self.citations:
            if counts[resource] > MAX_UNANCHORED_CITATIONS and anchor is None:
                self.err(cid, f"resource {resource!r} is cited {counts[resource]} times; this citation needs an anchor")

        # index.md files: present in every directory and without frontmatter
        for d in sorted({p.parent for p in BUNDLE.rglob("*.md")} | {BUNDLE}):
            idx = d / "index.md"
            if not idx.exists():
                self.errors.append(f"{idx.relative_to(BUNDLE)}: index.md is missing")
            elif idx.read_text(encoding="utf-8").startswith("---"):
                self.errors.append(f"{idx.relative_to(BUNDLE)}: index.md must not have frontmatter")
            else:
                text = idx.read_text(encoding="utf-8")
                for p in sorted(d.glob("*.md")):
                    if p.name in ("index.md", "log.md"):
                        continue
                    if f"({p.name})" not in text:
                        self.errors.append(f"{idx.relative_to(BUNDLE)}: does not list {p.name}")
        if not (BUNDLE / "log.md").exists():
            self.errors.append("log.md is missing at the bundle root")

    def run(self) -> list[str]:
        for cid, d in self.c.items():
            self.check_concept(cid, d)
        self.check_bundle()
        return self.errors


def summary(concepts: dict[str, dict]) -> str:
    by_type = Counter(d.get("type") for d in concepts.values())
    by_tier = Counter(d.get("tier", "-") for d in concepts.values())
    by_basis = Counter(d.get("basis", "-") for d in concepts.values())
    lines = [f"{len(concepts)} concepts"]
    lines.append("  by type: " + ", ".join(f"{k} {v}" for k, v in sorted(by_type.items())))
    lines.append("  by tier: " + ", ".join(f"{k} {v}" for k, v in sorted(by_tier.items())))
    lines.append("  by basis: " + ", ".join(f"{k} {v}" for k, v in sorted(by_basis.items())))
    return "\n".join(lines)


def main(argv: list[str]) -> int:
    global BUNDLE
    if len(argv) > 1:
        BUNDLE = Path(argv[1]).resolve()
    try:
        concepts = load_bundle(BUNDLE)
    except ValueError as e:
        print(f"FAIL {e}")
        return 1
    errors = Checker(concepts).run()
    print(summary(concepts))
    if errors:
        print(f"\n{len(errors)} problems:")
        for e in errors:
            print("  " + e)
        return 1
    print("\nOK")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
