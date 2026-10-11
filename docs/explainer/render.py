#!/usr/bin/env python3
"""Render the explainer bundle to one self-contained HTML page.

Stage 3 of the explainer. Everything on the page comes from the bundle under
./bundle; nothing is hand-written here except the page's skeleton. Styling is
read from explainer.css and inlined at build time, and every diagram is
generated from frontmatter as Mermaid text.

Usage: python3 docs/explainer/render.py [bundle-dir] [out.html]
"""

from __future__ import annotations

import datetime as dt
import html
import re
import subprocess
import sys
from collections import defaultdict
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from check import BUNDLE as DEFAULT_BUNDLE  # noqa: E402
from check import REPO, load_bundle  # noqa: E402

HERE = Path(__file__).resolve().parent
CSS = HERE / "explainer.css"
OUT = HERE / "explainer.html"
MERMAID = "https://cdn.jsdelivr.net/npm/mermaid@11.4.1/dist/mermaid.min.js"

SECTIONS = [
    ("term", "Terms"),
    ("component", "Components"),
    ("workflow", "Workflows"),
    ("data-structure", "Data structures"),
    ("decision", "Decisions"),
    ("invariant", "Invariants"),
    ("change-path", "Change paths"),
    ("gap", "Gaps"),
]
TIER_ORDER = {"core": 0, "supporting": 1, "detail": 2}
MAX_NODES = 10


# --------------------------------------------------------------------------- helpers


def slug(cid: str) -> str:
    return cid.strip("/").removesuffix(".md").replace("/", "-")


def esc(s) -> str:
    return html.escape(str(s), quote=True)


def palette(css_text: str) -> dict[str, str]:
    """Type colours are defined once, in explainer.css, as --type-<name>."""
    return dict(re.findall(r"--type-([a-z-]+):\s*(#[0-9a-fA-F]{3,6})", css_text))


def git_short(commit: str) -> str:
    return commit[:10]


def git_date(commit: str) -> str:
    try:
        return subprocess.run(
            ["git", "-C", str(REPO), "show", "-s", "--format=%cs", commit],
            capture_output=True, text=True, check=True,
        ).stdout.strip()
    except subprocess.CalledProcessError:
        return "?"


# --------------------------------------------------------------------------- markdown


class Markdown:
    """A small Markdown-to-HTML converter for the bundle's bodies: headings,
    paragraphs, lists, tables, fenced code, quotes, and inline code, emphasis
    and links. Every link to a concept, and every mention of a concept's
    title, becomes a link to that concept."""

    def __init__(self, concepts: dict[str, dict]):
        self.c = concepts
        titles = sorted(((d["title"], cid) for cid, d in concepts.items()), key=lambda t: -len(t[0]))
        self.title_re = re.compile(
            r"(?<![\w/`-])(" + "|".join(re.escape(t) for t, _ in titles) + r")(?![\w-])"
        ) if titles else None
        self.by_title = {t: cid for t, cid in titles}

    # inline ---------------------------------------------------------------
    def inline(self, text: str, autolink: bool = True) -> str:
        parts = re.split(r"(`[^`]*`)", text)
        out = []
        for i, p in enumerate(parts):
            if i % 2 == 1:
                out.append("<code>" + esc(p[1:-1]) + "</code>")
            else:
                out.append(self._inline_text(p, autolink))
        return "".join(out)

    def _inline_text(self, text: str, autolink: bool) -> str:
        # links first, protecting their text from autolinking
        pieces = re.split(r"(\[[^\]]+\]\([^)]+\))", text)
        out = []
        for i, p in enumerate(pieces):
            if i % 2 == 1:
                m = re.fullmatch(r"\[([^\]]+)\]\(([^)]+)\)", p)
                label, target = m.group(1), m.group(2)
                out.append(self._link(label, target))
            else:
                s = esc(p)
                s = re.sub(r"\*\*([^*]+)\*\*", r"<strong>\1</strong>", s)
                s = re.sub(r"(?<![\w*])\*([^*\n]+)\*(?![\w*])", r"<em>\1</em>", s)
                if autolink and self.title_re is not None:
                    s = self.title_re.sub(self._autolink, s)
                out.append(s)
        return "".join(out)

    def _autolink(self, m: re.Match) -> str:
        cid = self.by_title[m.group(1)]
        return f'<a class="ref ref-{self.c[cid]["type"]}" href="#{slug(cid)}">{m.group(1)}</a>'

    def _link(self, label: str, target: str) -> str:
        t = target.split("#")[0]
        if t in self.c:
            cls = f'ref ref-{self.c[t]["type"]}'
            return f'<a class="{cls}" href="#{slug(t)}">{self.inline(label, autolink=False)}</a>'
        return f'<a href="{esc(target)}">{self.inline(label, autolink=False)}</a>'

    # blocks ---------------------------------------------------------------
    def render(self, text: str, heading_shift: int = 2) -> str:
        lines = text.splitlines()
        out: list[str] = []
        i = 0
        para: list[str] = []

        def flush_para():
            if para:
                out.append("<p>" + self.inline(" ".join(s.strip() for s in para)) + "</p>")
                para.clear()

        while i < len(lines):
            line = lines[i]
            s = line.strip()
            if not s:
                flush_para()
                i += 1
                continue
            if s.startswith("```"):
                flush_para()
                lang = s[3:].strip()
                code = []
                i += 1
                while i < len(lines) and not lines[i].strip().startswith("```"):
                    code.append(lines[i])
                    i += 1
                i += 1
                cls = f' class="lang-{esc(lang)}"' if lang else ""
                out.append(f"<pre><code{cls}>" + esc("\n".join(code)) + "</code></pre>")
                continue
            m = re.match(r"^(#{1,6})\s+(.*)$", s)
            if m:
                flush_para()
                level = min(6, len(m.group(1)) + heading_shift)
                out.append(f"<h{level}>{self.inline(m.group(2))}</h{level}>")
                i += 1
                continue
            if s.startswith("|"):
                flush_para()
                rows = []
                while i < len(lines) and lines[i].strip().startswith("|"):
                    rows.append(lines[i].strip())
                    i += 1
                out.append(self._table(rows))
                continue
            if s.startswith(">"):
                flush_para()
                q = []
                while i < len(lines) and lines[i].strip().startswith(">"):
                    q.append(lines[i].strip()[1:].strip())
                    i += 1
                out.append("<blockquote>" + self.render("\n".join(q), heading_shift) + "</blockquote>")
                continue
            if re.match(r"^(\s*)([-*]|\d+\.)\s+", line):
                flush_para()
                items = []
                while i < len(lines) and (re.match(r"^(\s*)([-*]|\d+\.)\s+", lines[i]) or (lines[i].startswith("  ") and lines[i].strip())):
                    items.append(lines[i])
                    i += 1
                out.append(self._list(items))
                continue
            para.append(line)
            i += 1
        flush_para()
        return "\n".join(out)

    def _table(self, rows: list[str]) -> str:
        def cells(r: str) -> list[str]:
            r = r.strip().strip("|")
            return [c.strip() for c in re.split(r"(?<!\\)\|", r)]
        head = cells(rows[0])
        body = [cells(r) for r in rows[2:]] if len(rows) > 1 and re.fullmatch(r"[|\s:-]+", rows[1]) else [cells(r) for r in rows[1:]]
        h = "".join(f"<th>{self.inline(c)}</th>" for c in head)
        b = "".join("<tr>" + "".join(f"<td>{self.inline(c)}</td>" for c in r) + "</tr>" for r in body)
        return f'<div class="scroll"><table><thead><tr>{h}</tr></thead><tbody>{b}</tbody></table></div>'

    def _list(self, items: list[str]) -> str:
        # one or two levels, by indentation
        first = re.match(r"^(\s*)([-*]|\d+\.)\s+", items[0])
        ordered = first.group(2) != "-" and first.group(2) != "*"
        tag = "ol" if ordered else "ul"
        out = [f"<{tag}>"]
        open_li = False
        sub: list[str] = []

        def close_li():
            nonlocal open_li, sub
            if sub:
                out.append(self._list(sub))
                sub = []
            if open_li:
                out.append("</li>")
                open_li = False

        base = len(first.group(1))
        for it in items:
            m = re.match(r"^(\s*)([-*]|\d+\.)\s+(.*)$", it)
            if m and len(m.group(1)) <= base:
                close_li()
                out.append("<li>" + self.inline(m.group(3)))
                open_li = True
            elif m:
                sub.append(it[base + 2 :] if it.startswith(" " * (base + 2)) else it.lstrip())
            else:
                # continuation line
                if sub:
                    sub.append(it.strip())
                else:
                    out.append(" " + self.inline(it.strip()))
        close_li()
        out.append(f"</{tag}>")
        return "\n".join(out)


# --------------------------------------------------------------------------- diagrams


def mm_id(cid: str) -> str:
    return "n_" + re.sub(r"[^A-Za-z0-9]", "_", slug(cid))


def mm_label(text: str) -> str:
    return '"' + text.replace('"', "'") + '"'


def diagram_block(title: str, mermaid: str, note: str = "") -> str:
    n = f'<p class="diagram-note">{esc(note)}</p>' if note else ""
    return (
        f'<figure class="diagram"><figcaption>{esc(title)}</figcaption>{n}'
        f'<div class="scroll"><pre class="mermaid">{esc(mermaid)}</pre></div></figure>'
    )


def class_defs(colours: dict[str, str], used: set[str]) -> list[str]:
    out = []
    for t in sorted(used):
        c = colours.get(t, "#999999")
        out.append(f"classDef {t.replace('-', '_')} fill:{c}22,stroke:{c},color:#111,stroke-width:1.5px;")
    return out


def component_diagrams(c: dict[str, dict], colours: dict[str, str]) -> tuple[str, str]:
    """The overview diagram for the map, and the detail diagrams for the
    components section (empty when one diagram holds every component)."""
    comps = {cid: d for cid, d in c.items() if d["type"] == "component"}
    if len(comps) < 3:
        return "", ""

    def graph(nodes: dict[str, dict], title: str, note: str = "", externals: set[str] = frozenset()) -> str:
        lines = ["graph LR"]
        for cid in nodes:
            lines.append(f"  {mm_id(cid)}[{mm_label(nodes[cid]['title'])}]")
        for cid in externals:
            lines.append(f"  {mm_id(cid)}[{mm_label(c[cid]['title'])}]")
        for cid, d in nodes.items():
            for dep in d.get("depends_on") or []:
                if dep in nodes or dep in externals:
                    lines.append(f"  {mm_id(cid)} --> {mm_id(dep)}")
        for cid in nodes:
            lines.append(f"  click {mm_id(cid)} \"#{slug(cid)}\"")
        for cid in externals:
            lines.append(f"  click {mm_id(cid)} \"#{slug(cid)}\"")
        lines += class_defs(colours, {"component"})
        lines.append("  class " + ",".join(mm_id(x) for x in list(nodes) + list(externals)) + " component;")
        return diagram_block(title, "\n".join(lines), note)

    if len(comps) <= MAX_NODES:
        return graph(comps, "Components and what they depend on"), ""

    # Overview by group, detail per group. `group` is an optional component key.
    groups: dict[str, dict[str, dict]] = defaultdict(dict)
    for cid, d in comps.items():
        groups[d.get("group") or "ungrouped"][cid] = d
    gid = {g: "g_" + re.sub(r"[^A-Za-z0-9]", "_", g) for g in groups}
    lines = ["graph LR"]
    for g, members in groups.items():
        lines.append(f"  {gid[g]}[{mm_label(g + ' (' + str(len(members)) + ')')}]")
    edges = set()
    for g, members in groups.items():
        for cid, d in members.items():
            for dep in d.get("depends_on") or []:
                if dep in comps:
                    dg = comps[dep].get("group") or "ungrouped"
                    if dg != g:
                        edges.add((g, dg))
    for a, b in sorted(edges):
        lines.append(f"  {gid[a]} --> {gid[b]}")
    lines += class_defs(colours, {"component"})
    lines.append("  class " + ",".join(gid.values()) + " component;")
    overview = diagram_block(
        "Components: overview by group",
        "\n".join(lines),
        f"{len(comps)} components, so the map shows the groups; each group's detail is in the components section.",
    )
    out = []
    for g, members in groups.items():
        ext = set()
        for d in members.values():
            for dep in d.get("depends_on") or []:
                if dep in comps and dep not in members:
                    ext.add(dep)
        out.append(graph(members, f"Components: {g}", "Boxes outside the group are drawn where a member depends on them.", ext))
    return overview, "\n".join(out)


def workflow_diagram(c: dict[str, dict], wf: dict, colours: dict[str, str], overview_only: bool = False) -> str:
    """The diagram the table in the instructions prescribes. With overview_only,
    a long workflow gives just its overview (the map's version)."""
    steps = [s for s in wf.get("steps") or [] if isinstance(s, dict)]
    if any("branch" in s for s in steps):
        out = flowchart(c, wf, steps, colours)
    else:
        actors = {s["actor"] for s in steps} | {s["caller"] for s in steps if s.get("caller") not in (None, "outside")}
        if len(actors) < 2:
            return ""
        out = sequence(c, wf, steps, colours)
    if overview_only and len(steps) > MAX_NODES:
        return out.split("</figure>", 1)[0] + "</figure>"
    return out


def flowchart(c: dict[str, dict], wf: dict, steps: list[dict], colours: dict[str, str]) -> str:
    if len(steps) < 3:
        return ""

    def one(sub: list[tuple[int, dict]], title: str, note: str = "") -> str:
        lines = ["flowchart TD"]
        ids = {}
        for n, s in sub:
            sid = f"s{n}"
            ids[n] = sid
            label = mm_label(f"({n}) {s['label']}")
            if s.get("ends"):
                lines.append(f"  {sid}([{label}])")
            else:
                lines.append(f"  {sid}[{label}]")
        # Each step follows the nearest earlier step that does not end the
        # workflow; a step's `branch` labels the edge into it. So a fork reads
        # as: the deciding step, then one or more branch steps, the ending
        # ones hanging off it and the continuing one carrying on.
        last_open = None
        for n, s in sub:
            if last_open is not None:
                br = s.get("branch")
                if br:
                    lines.append(f"  {ids[last_open]} -->|{mm_label(br)}| {ids[n]}")
                else:
                    lines.append(f"  {ids[last_open]} --> {ids[n]}")
            if not s.get("ends"):
                last_open = n
        lines += class_defs(colours, {"workflow"})
        lines.append("  class " + ",".join(ids.values()) + " workflow;")
        return diagram_block(title, "\n".join(lines), note)

    numbered = list(enumerate(steps, 1))
    if len(numbered) <= MAX_NODES:
        return one(numbered, f"{wf['title']}: flow")
    out = []
    # overview: runs of consecutive steps by actor
    lines = ["flowchart TD"]
    runs: list[tuple[str, int, int]] = []
    for n, s in numbered:
        if runs and runs[-1][0] == s["actor"]:
            runs[-1] = (runs[-1][0], runs[-1][1], n)
        else:
            runs.append((s["actor"], n, n))
    for k, (actor, a, b) in enumerate(runs):
        rng = f"step {a}" if a == b else f"steps {a}–{b}"
        lines.append(f"  r{k}[{mm_label(c[actor]['title'] + ' (' + rng + ')')}]")
    for k in range(len(runs) - 1):
        lines.append(f"  r{k} --> r{k + 1}")
    lines += class_defs(colours, {"component"})
    lines.append("  class " + ",".join(f"r{k}" for k in range(len(runs))) + " component;")
    out.append(diagram_block(f"{wf['title']}: overview", "\n".join(lines), f"{len(steps)} steps, so the overview groups consecutive steps by actor; the detail follows in parts."))
    for k in range(0, len(numbered), MAX_NODES):
        part = numbered[k : k + MAX_NODES]
        out.append(one(part, f"{wf['title']}: steps {part[0][0]} to {part[-1][0]}"))
    return "\n".join(out)


def sequence(c: dict[str, dict], wf: dict, steps: list[dict], colours: dict[str, str]) -> str:
    order: list[str] = []
    for s in steps:
        for p in (s.get("caller"), s["actor"]):
            if p and p not in order:
                order.append(p)
    if len(order) < 3 and not any(s.get("caller") for s in steps):
        return ""

    def pid(p: str) -> str:
        return "outside" if p == "outside" else mm_id(p)

    def one(sub: list[tuple[int, dict]], title: str, note: str = "") -> str:
        lines = ["sequenceDiagram"]
        parts: list[str] = []
        for n, s in sub:
            for p in (s.get("caller"), s["actor"]):
                if p and p not in parts:
                    parts.append(p)
        for p in parts:
            name = "outside" if p == "outside" else c[p]["title"]
            lines.append(f"  participant {pid(p)} as {name}")
        for n, s in sub:
            label = f"({n}) {s['label']}"
            if s.get("caller"):
                lines.append(f"  {pid(s['caller'])}->>{pid(s['actor'])}: {label}")
            else:
                lines.append(f"  Note over {pid(s['actor'])}: {label}")
        return diagram_block(title, "\n".join(lines), note)

    numbered = list(enumerate(steps, 1))
    out = []
    if len(numbered) > MAX_NODES:
        # Overview: one arrow per run of consecutive steps with the same caller
        # and actor, labelled with the step numbers it stands for. The full
        # sequence, every label in full, follows.
        lines = ["sequenceDiagram"]
        for p in order:
            lines.append(f"  participant {pid(p)} as {'outside' if p == 'outside' else c[p]['title']}")
        runs: list[tuple[str | None, str, int, int]] = []
        for n, st in numbered:
            key = (st.get("caller"), st["actor"])
            if runs and (runs[-1][0], runs[-1][1]) == key:
                runs[-1] = (key[0], key[1], runs[-1][2], n)
            else:
                runs.append((key[0], key[1], n, n))
        for caller, actor, a, b in runs:
            text = f"step {a}" if a == b else f"steps {a} to {b}"
            if caller:
                lines.append(f"  {pid(caller)}->>{pid(actor)}: {text}")
            else:
                lines.append(f"  Note over {pid(actor)}: {text}")
        out.append(diagram_block(f"{wf['title']}: overview", "\n".join(lines),
                                 f"{len(numbered)} steps, so this shows who calls whom; the full sequence with every step is in the workflow's entry."))
    if len(order) <= MAX_NODES:
        out.append(one(numbered, f"{wf['title']}: sequence"))
    else:
        out.append(one(numbered, f"{wf['title']}: sequence", f"{len(order)} participants; the parts below show at most {MAX_NODES} each."))
        for k in range(0, len(numbered), MAX_NODES):
            part = numbered[k : k + MAX_NODES]
            out.append(one(part, f"{wf['title']}: steps {part[0][0]} to {part[-1][0]}"))
    return "\n".join(out)


def er_diagrams(c: dict[str, dict], colours: dict[str, str]) -> str:
    ds = {cid: d for cid, d in c.items() if d["type"] == "data-structure"}
    if len(ds) < 3:
        return ""
    CARD = {"1:1": "||--||", "1:n": "||--o{", "n:1": "}o--||", "n:n": "}o--o{"}

    def ename(cid: str) -> str:
        return re.sub(r"[^A-Za-z0-9_]", "_", ds[cid]["title"]).strip("_")

    def one(nodes: set[str], title: str, note: str = "", members: set[str] | None = None) -> str:
        lines = ["erDiagram"]
        for cid in sorted(nodes):
            lines.append(f"  {ename(cid)} {{ }}")
        for cid in sorted(members if members is not None else nodes):
            for r in ds[cid].get("references") or []:
                if r.get("to") in nodes:
                    how = str(r.get("how", "")).replace('"', "'")
                    lines.append(f"  {ename(cid)} {CARD.get(r.get('card'), '||--||')} {ename(r['to'])} : \"{how}\"")
        return diagram_block(title, "\n".join(lines), note)

    if len(ds) <= MAX_NODES:
        return one(set(ds), "Data structures and how they refer to each other")

    # Overview by group, detail per group. `group` is an optional key.
    groups: dict[str, set[str]] = defaultdict(set)
    for cid, d in ds.items():
        groups[d.get("group") or "ungrouped"].add(cid)
    lines = ["erDiagram"]
    for g in groups:
        lines.append(f"  {re.sub(r'[^A-Za-z0-9_]', '_', g)} {{ }}")
    seen: set[tuple[str, str]] = set()
    for g, members in groups.items():
        for cid in members:
            for r in ds[cid].get("references") or []:
                tg = ds.get(r.get("to"), {}).get("group") or "ungrouped"
                if r.get("to") in ds and tg != g and (g, tg) not in seen:
                    seen.add((g, tg))
                    a, b = (re.sub(r'[^A-Za-z0-9_]', '_', x) for x in (g, tg))
                    lines.append(f"  {a} }}o--o{{ {b} : \"refers to\"")
    out = [diagram_block("Data structures: overview by group", "\n".join(lines),
                         f"{len(ds)} data structures, so the overview shows the groups; each group's detail follows, with the structures it refers to outside it.")]
    for g, members in groups.items():
        ext = set()
        for cid in members:
            for r in ds[cid].get("references") or []:
                if r.get("to") in ds and r["to"] not in members:
                    ext.add(r["to"])
        if len(members | ext) < 3:
            continue
        out.append(one(members | ext, f"Data structures: {g}", "Boxes outside the group are drawn where a member refers to them.", members))
    return "\n".join(out)


# --------------------------------------------------------------------------- page pieces


def labels(d: dict) -> str:
    out = [f'<span class="label label-type type-{d["type"]}">{esc(d["type"])}</span>']
    if d.get("tier"):
        out.append(f'<span class="label label-tier">{esc(d["tier"])}</span>')
    if d.get("basis"):
        cls = " label-flag" if d["basis"] in ("inferred", "local") else ""
        out.append(f'<span class="label label-basis{cls}">basis: {esc(d["basis"])}</span>')
    if d.get("resource"):
        out.append(f'<span class="label label-resource"><code>{esc(d["resource"])}</code></span>')
    if d.get("anchor"):
        out.append(f'<span class="label label-anchor">anchor <code>{esc(d["anchor"])}</code></span>')
    return '<p class="labels">' + " ".join(out) + "</p>"


def flag(d: dict) -> str:
    if d.get("basis") in ("inferred", "local"):
        return f' <span class="flag">{esc(d["basis"])}</span>'
    return ""


def inbound(c: dict[str, dict]) -> dict[str, dict[str, set[str]]]:
    """For every concept: the workflows that touch it, the decisions that affect
    it, the invariants that apply to it, and other concepts that link to it."""
    inb: dict[str, dict[str, set[str]]] = defaultdict(lambda: defaultdict(set))
    link_re = re.compile(r"\]\((/[^)\s#]+\.md)")
    for cid, d in c.items():
        t = d["type"]
        if t == "workflow":
            for s in d.get("steps") or []:
                if not isinstance(s, dict):
                    continue
                for x in [s.get("actor"), s.get("caller")] + list(s.get("reads") or []) + list(s.get("writes") or []):
                    if x and x != "outside" and x in c:
                        inb[x]["workflows"].add(cid)
        elif t == "decision":
            for x in d.get("affects") or []:
                inb[x]["decisions"].add(cid)
        elif t == "invariant":
            for x in d.get("applies_to") or []:
                inb[x]["invariants"].add(cid)
        elif t == "change-path":
            for x in d.get("touches") or []:
                inb[x]["change paths"].add(cid)
        elif t == "component":
            for x in d.get("depends_on") or []:
                inb[x]["depended on by"].add(cid)
        elif t == "data-structure":
            for r in d.get("references") or []:
                if isinstance(r, dict) and r.get("to"):
                    inb[r["to"]]["referenced by"].add(cid)
        for m in link_re.finditer(d.get("_body", "")):
            tgt = m.group(1)
            if tgt in c and tgt != cid:
                inb[tgt]["mentioned by"].add(cid)
    return inb


def ref(c: dict[str, dict], cid: str) -> str:
    d = c[cid]
    return f'<a class="ref ref-{d["type"]}" href="#{slug(cid)}">{esc(d["title"])}</a>'


def steps_table(c: dict[str, dict], md: Markdown, steps: list[dict]) -> str:
    rows = []
    for n, s in enumerate(steps, 1):
        caller = "outside" if s.get("caller") == "outside" else (ref(c, s["caller"]) if s.get("caller") else "—")
        reads = ", ".join(ref(c, x) for x in s.get("reads") or []) or "—"
        writes = ", ".join(ref(c, x) for x in s.get("writes") or []) or "—"
        extra = []
        if s.get("branch"):
            extra.append(f'<span class="label">branch: {esc(s["branch"])}</span>')
        if s.get("ends"):
            extra.append('<span class="label">ends</span>')
        res = f'<code>{esc(s["resource"])}</code>'
        if s.get("anchor"):
            res += f'<br><small>anchor <code>{esc(s["anchor"])}</code></small>'
        rows.append(
            "<tr>"
            f"<td>{n}</td><td>{caller}</td><td>{ref(c, s['actor'])}</td>"
            f"<td>{md.inline(s['action'])} {' '.join(extra)}</td>"
            f"<td>{reads}</td><td>{writes}</td><td>{res}</td></tr>"
        )
    return (
        '<div class="scroll"><table class="steps"><thead><tr><th>#</th><th>caller</th><th>actor</th>'
        "<th>action</th><th>reads</th><th>writes</th><th>resource</th></tr></thead><tbody>"
        + "".join(rows)
        + "</tbody></table></div>"
    )


def meta_block(c: dict[str, dict], md: Markdown, d: dict) -> str:
    t = d["type"]
    out = []
    if t == "decision":
        alts = "".join(f"<li>{md.inline(str(a))}</li>" for a in d.get("alternatives") or [])
        out.append(f"<h4>Alternatives</h4><ul>{alts}</ul>")
        out.append("<p><strong>Affects</strong> " + ", ".join(ref(c, x) for x in d.get("affects") or []) + "</p>")
    elif t == "invariant":
        out.append("<p><strong>Applies to</strong> " + ", ".join(ref(c, x) for x in d.get("applies_to") or []) + "</p>")
        ea = d.get("enforced_at")
        if ea == "not enforced":
            out.append('<p><strong>Enforced at</strong> <span class="flag">not enforced</span></p>')
        else:
            a = f' (anchor <code>{esc(d["enforced_anchor"])}</code>)' if d.get("enforced_anchor") else ""
            out.append(f"<p><strong>Enforced at</strong> <code>{esc(ea)}</code>{a}</p>")
    elif t == "component":
        deps = ", ".join(ref(c, x) for x in d.get("depends_on") or []) or "nothing in the bundle"
        g = f' <span class="label">group: {esc(d["group"])}</span>' if d.get("group") else ""
        out.append(f"<p><strong>Depends on</strong> {deps}{g}</p>")
    elif t == "workflow":
        out.append(f"<p><strong>Trigger</strong> {md.inline(str(d.get('trigger', '')))}</p>")
    elif t == "data-structure":
        refs = d.get("references") or []
        if refs:
            rows = "".join(
                f"<tr><td>{esc(r.get('how', ''))}</td><td>{ref(c, r['to'])}</td><td>{esc(r.get('card', ''))}</td></tr>"
                for r in refs if isinstance(r, dict) and r.get("to") in c
            )
            out.append(f'<div class="scroll"><table><thead><tr><th>relation</th><th>to</th><th>cardinality</th></tr></thead><tbody>{rows}</tbody></table></div>')
    elif t == "change-path":
        out.append("<p><strong>Touches</strong> " + ", ".join(ref(c, x) for x in d.get("touches") or []) + "</p>")
    elif t == "gap":
        la = d.get("looked_at")
        items = la if isinstance(la, list) else [la]
        out.append("<p><strong>Looked at</strong> " + "; ".join(f"<code>{esc(x)}</code>" for x in items) + "</p>")
    elif t == "purpose":
        out.append(f"<p><strong>Primary workflow</strong> {ref(c, d['primary_workflow'])}</p>")
    if d.get("tested_by"):
        out.append("<p><strong>Tested by</strong> " + ", ".join(f"<code>{esc(x)}</code>" for x in d["tested_by"]) + "</p>")
    return "\n".join(out)


def used_block(c: dict[str, dict], inb: dict, cid: str) -> str:
    groups = inb.get(cid)
    if not groups:
        return ""
    parts = []
    for name in ("workflows", "decisions", "invariants", "change paths", "depended on by", "referenced by", "mentioned by"):
        if groups.get(name):
            parts.append(f"<li><strong>{name}</strong> " + ", ".join(ref(c, x) for x in sorted(groups[name])) + "</li>")
    return '<div class="used"><h4>Where else it is used</h4><ul>' + "".join(parts) + "</ul></div>"


def concept_html(c: dict[str, dict], md: Markdown, inb: dict, cid: str, colours: dict[str, str], force_open: bool = False) -> str:
    d = c[cid]
    is_open = d.get("tier") == "core" or force_open
    body = md.render(d.get("_body", ""))
    extra = ""
    if d["type"] == "workflow":
        extra = workflow_diagram(c, d, colours) + steps_table(c, md, [s for s in d.get("steps") or [] if isinstance(s, dict)])
    inner = labels(d) + meta_block(c, md, d) + extra + body + used_block(c, inb, cid)
    return (
        f'<details class="concept type-{d["type"]}" id="{slug(cid)}"{" open" if is_open else ""}>'
        f'<summary><span class="title">{esc(d["title"])}</span>{flag(d)} <span class="desc">{md.inline(d.get("description", ""))}</span></summary>'
        f'<div class="concept-body">{inner}</div></details>'
    )


def sort_key(c: dict[str, dict]):
    return lambda cid: (TIER_ORDER.get(c[cid].get("tier", "supporting"), 1), c[cid]["title"].lower())


# --------------------------------------------------------------------------- page


def build(bundle: Path, out: Path) -> None:
    c = load_bundle(bundle)
    css_text = CSS.read_text(encoding="utf-8")
    colours = palette(css_text)
    md = Markdown(c)
    inb = inbound(c)
    purposes = [cid for cid, d in c.items() if d["type"] == "purpose"]
    purpose = c[purposes[0]] if purposes else {"title": "?", "description": "", "commit": "", "_body": ""}
    commit = purpose.get("commit", "")
    repo_name = REPO.name
    by_type: dict[str, list[str]] = defaultdict(list)
    for cid, d in c.items():
        by_type[d["type"]].append(cid)
    for t in by_type:
        by_type[t].sort(key=sort_key(c))
    core = sorted((cid for cid, d in c.items() if d.get("tier") == "core"), key=lambda x: (SECTIONS_INDEX.get(c[x]["type"], 99), c[x]["title"]))
    inferred = [cid for cid, d in c.items() if d.get("basis") == "inferred"]
    local = [cid for cid, d in c.items() if d.get("basis") == "local"]
    gaps = by_type.get("gap", [])
    generated = dt.date.today().isoformat()

    # purpose: the "does not do" list is the body's section headed "What it does not do"
    body = purpose.get("_body", "")
    m = re.search(r"^#+\s+What it (?:deliberately )?does(?:n't| not) do\s*$([\s\S]*?)(?=^#+\s|\Z)", body, re.MULTILINE)
    not_do = md.render(m.group(1)) if m else ""

    # ---- map
    legend = " ".join(f'<span class="label label-type type-{t}">{esc(name)}</span>' for t, name in [("purpose", "purpose")] + SECTIONS)
    map_html = [
        '<section id="map" class="map">',
        f"<header><h1>{esc(repo_name)} explainer</h1>",
        f'<p class="meta">commit <code>{esc(git_short(commit))}</code> ({esc(git_date(commit))}) · generated {generated} · '
        f'{len(c)} concepts · {len(inferred)} inferred, {len(local)} local · {len(gaps)} gaps</p>',
        f'<p class="legend">{legend} <button type="button" id="expand-all">Expand all</button></p></header>',
        f'<h2 id="purpose-heading">{esc(purpose["title"])}</h2>',
        f'<p class="purpose-desc">{md.inline(purpose.get("description", ""))}</p>',
        f'<div class="not-do"><h3>What it does not do</h3>{not_do}</div>' if not_do else "",
        '<h2 id="core">Core concepts</h2><ul class="core-list">',
    ]
    for cid in core:
        d = c[cid]
        map_html.append(f'<li>{ref(c, cid)}{flag(d)} <span class="desc">{md.inline(d.get("description", ""))}</span></li>')
    map_html.append("</ul>")
    comp_overview, comp_details = component_diagrams(c, colours)
    map_html.append('<h2 id="component-map">Components</h2>' + comp_overview)
    pw = purpose.get("primary_workflow")
    if pw in c:
        map_html.append(f'<h2 id="primary-workflow">Primary workflow: {ref(c, pw)}</h2>')
        map_html.append(f'<p>{md.inline(c[pw].get("description", ""))}</p>')
        map_html.append(workflow_diagram(c, c[pw], colours, overview_only=True))
    map_html.append('<h2 id="gap-map">Gaps</h2><ul class="gap-list">')
    for cid in gaps:
        map_html.append(f'<li>{ref(c, cid)}</li>')
    map_html.append("</ul></section>")

    # ---- reference
    ref_html = ['<section id="reference" class="reference">']
    ref_html.append(f'<h2 id="sec-purpose">Purpose</h2>' + "".join(concept_html(c, md, inb, cid, colours, force_open=True) for cid in purposes))
    for t, name in SECTIONS:
        ids = by_type.get(t, [])
        ref_html.append(f'<h2 id="sec-{t}">{esc(name)} <small>({len(ids)})</small></h2>')
        if t == "component":
            ref_html.append(comp_details)
        if t == "data-structure":
            ref_html.append(er_diagrams(c, colours))
        for cid in ids:
            ref_html.append(concept_html(c, md, inb, cid, colours))
    log = bundle / "log.md"
    if log.exists():
        ref_html.append('<h2 id="sec-log">Recent changes</h2><div class="log">' + md.render(log.read_text(encoding="utf-8"), heading_shift=2) + "</div>")
    ref_html.append("</section>")

    # ---- nav
    nav = ['<nav class="side"><p class="nav-title">On this page</p><ul>',
           '<li><a href="#map">Map</a></li>', '<li><a href="#core">Core concepts</a></li>',
           '<li><a href="#component-map">Component diagram</a></li>', '<li><a href="#primary-workflow">Primary workflow</a></li>',
           '<li><a href="#gap-map">Gaps</a></li>', '<li><a href="#sec-purpose">Purpose</a></li>']
    for t, name in SECTIONS:
        nav.append(f'<li><a href="#sec-{t}" class="ref-{t}">{esc(name)}</a></li>')
    nav.append('<li><a href="#sec-log">Recent changes</a></li></ul>')
    nav.append('<p class="nav-title">Core</p><ul>' + "".join(f"<li>{ref(c, cid)}</li>" for cid in core) + "</ul></nav>")

    script = """
(function () {
  function openTarget() {
    var id = decodeURIComponent(location.hash.slice(1));
    if (!id) return;
    var el = document.getElementById(id);
    if (!el) return;
    var d = el.closest ? el.closest('details') : null;
    while (d) { d.open = true; d = d.parentElement && d.parentElement.closest('details'); }
    if (el.tagName === 'DETAILS') el.open = true;
    el.scrollIntoView();
  }
  window.addEventListener('hashchange', openTarget);
  window.addEventListener('load', openTarget);
  var all = false;
  document.getElementById('expand-all').addEventListener('click', function () {
    all = !all;
    document.querySelectorAll('details.concept').forEach(function (d) { d.open = all; });
    this.textContent = all ? 'Collapse all' : 'Expand all';
  });
  mermaid.initialize({
    startOnLoad: true, securityLevel: 'loose', theme: 'base',
    flowchart: { useMaxWidth: false, htmlLabels: true },
    sequence: { useMaxWidth: false, wrap: false, actorFontSize: 14, messageFontSize: 14, noteFontSize: 14, messageMargin: 14, boxMargin: 6, noteMargin: 6, height: 36, actorMargin: 40, bottomMarginAdj: 1, mirrorActors: false },
    er: { useMaxWidth: false, fontSize: 14 },
    themeVariables: { fontSize: '14px', fontFamily: 'system-ui, sans-serif',
      actorBkg: '__ACTOR_BKG__', actorBorder: '__ACTOR_BORDER__', actorTextColor: '#111',
      noteBkgColor: '#f4f4f4', noteBorderColor: '#bbb', primaryTextColor: '#111', lineColor: '#555' }
  });
})();
""".replace("__ACTOR_BKG__", colours.get("component", "#999") + "22").replace("__ACTOR_BORDER__", colours.get("component", "#999"))

    page = f"""<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{esc(repo_name)} explainer</title>
<style>
{css_text}
</style>
<script src="{MERMAID}"></script>
</head>
<body>
{''.join(nav)}
<main>
{''.join(map_html)}
{''.join(ref_html)}
</main>
<script>{script}</script>
</body>
</html>
"""
    out.write_text(page, encoding="utf-8")
    print(f"wrote {out} ({len(page) // 1024} KB, {len(c)} concepts)")


SECTIONS_INDEX = {t: i for i, (t, _) in enumerate(SECTIONS)}


if __name__ == "__main__":
    b = Path(sys.argv[1]).resolve() if len(sys.argv) > 1 else DEFAULT_BUNDLE
    o = Path(sys.argv[2]).resolve() if len(sys.argv) > 2 else OUT
    build(b, o)
