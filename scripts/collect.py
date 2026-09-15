#!/usr/bin/env python3
"""Collect (diff, conventional-commit type) pairs from open-source repositories.

Stage 1: probe each repo through the GitHub API (last 100 commits) and keep
         repos whose conventional-commit ratio is >= PROBE_MIN_RATIO.
Stage 2: shallow bare clone, stream `git log -p`, split into commits, keep
         non-merge conventional commits, write one JSONL file per repo.

Output: data/raw/<owner>__<name>.jsonl.gz  (one JSON object per commit)
        data/probe.json                    (probe results)
"""
import gzip, json, os, re, subprocess, sys, time, hashlib, shutil
from concurrent.futures import ThreadPoolExecutor, as_completed

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
DATA = os.path.join(ROOT, "data")
RAW = os.path.join(DATA, "raw")
REPOS = os.path.join(DATA, "repos")
os.makedirs(RAW, exist_ok=True); os.makedirs(REPOS, exist_ok=True)

DEPTH = int(os.environ.get("DEPTH", "4000"))
MAX_COMMITS = int(os.environ.get("MAX_COMMITS", "5000"))
WORKERS = int(os.environ.get("WORKERS", "6"))
PROBE_MIN_RATIO = float(os.environ.get("PROBE_MIN_RATIO", "0.45"))
KEEP_CLONES = os.environ.get("KEEP_CLONES", "0") == "1"
MAX_FILES = 120           # commits touching more files are dropped
MAX_LINES_PER_FILE = 600  # patch lines kept per file (headers always kept)
MAX_PATCH_BYTES = 400_000

TYPES = {"feat","fix","docs","style","refactor","perf","test","build","ci","chore","revert"}
ALIASES = {"tests":"test","feature":"feat","bugfix":"fix","doc":"docs","refac":"refactor","perfs":"perf","bug":"fix"}
CC_RE = re.compile(r"^(?P<type>[A-Za-z]+)(?:\((?P<scope>[^)\n]*)\))?(?P<bang>!)?:\s+(?P<subject>.+)$")
# numpy/pandas/scipy/sklearn/matplotlib style: "ENH: ...", "BUG: ...", "DOC: ..."
SCIPY_MAP = {"ENH":"feat","FEA":"feat","FEAT":"feat","BUG":"fix","FIX":"fix","DOC":"docs","DOCS":"docs",
             "TST":"test","TEST":"test","MAINT":"chore","MNT":"chore","CLN":"refactor","REF":"refactor",
             "PERF":"perf","CI":"ci","BLD":"build","STY":"style","DEP":"build","DEPS":"build","TYP":"refactor"}
SCIPY_RE = re.compile(r"^(?P<type>[A-Z]{2,5})(?:\s*[:(])")
BOT_RE = re.compile(r"\[bot\]|dependabot|renovate|github-actions|semantic-release|greenkeeper|allcontributors|snyk-bot|pre-commit-ci|copybara|gcf-owl-bot|release-please|weblate|transifex", re.I)

def parse_subject(subject):
    """Return (type, scope, breaking, subject, convention) or None."""
    m = CC_RE.match(subject)
    if m:
        t = m.group("type").lower()
        t = ALIASES.get(t, t)
        if t in TYPES:
            return t, (m.group("scope") or "").strip(), bool(m.group("bang")), m.group("subject").strip(), "cc"
    m = SCIPY_RE.match(subject)
    if m and m.group("type") in SCIPY_MAP:
        rest = re.sub(r"^[A-Z]{2,5}\s*(\([^)]*\))?\s*:?\s*", "", subject).strip()
        return SCIPY_MAP[m.group("type")], "", False, rest, "scipy"
    return None

def log(*a):
    print(time.strftime("%H:%M:%S"), *a, flush=True)

# ---------------------------------------------------------------- stage 1
def probe(repo):
    try:
        out = subprocess.run(["gh","api","-X","GET",f"repos/{repo}/commits","-f","per_page=100"],
                             capture_output=True, text=True, timeout=60)
        if out.returncode != 0:
            return repo, {"error": out.stderr.strip()[:200]}
        commits = json.loads(out.stdout)
        subs = [c["commit"]["message"].split("\n",1)[0] for c in commits if len(c.get("parents",[]))<=1]
        n = len(subs)
        cc = sum(1 for s in subs if parse_subject(s) is not None)
        return repo, {"n": n, "cc": cc, "ratio": cc/max(n,1)}
    except Exception as e:
        return repo, {"error": str(e)[:200]}

def stage1(repos):
    path = os.path.join(DATA, "probe.json")
    res = json.load(open(path)) if os.path.exists(path) else {}
    todo = [r for r in repos if r not in res or "error" in res[r]]
    log(f"probe: {len(todo)} repos to probe ({len(res)} cached)")
    with ThreadPoolExecutor(8) as ex:
        for repo, r in ex.map(probe, todo):
            res[repo] = r
            log(f"probe {repo}: {r}")
            json.dump(res, open(path,"w"), indent=1)
    return res

# ---------------------------------------------------------------- stage 2
def run(cmd, cwd=None, timeout=None):
    return subprocess.run(cmd, cwd=cwd, capture_output=True, text=True, timeout=timeout, errors="replace")

def clone(repo):
    dest = os.path.join(REPOS, repo.replace("/","__") + ".git")
    if os.path.isdir(dest):
        return dest
    r = run(["git","clone","--bare","--single-branch","--quiet",f"--depth={DEPTH}",
             f"https://github.com/{repo}.git", dest], timeout=3600)
    if r.returncode != 0:
        shutil.rmtree(dest, ignore_errors=True)
        raise RuntimeError("clone failed: " + r.stderr.strip()[-300:])
    return dest

SEP = "\x1e"
def iter_commits(gitdir):
    """Yield (header_fields, patch_text) streaming git log -p."""
    cmd = ["git","log","--no-merges","--no-color",f"--max-count={MAX_COMMITS}",
           "-M","--find-renames","--no-decorate","--diff-algorithm=default",
           f"--format={SEP}%H%x00%an%x00%ae%x00%at%x00%s", "-p", "HEAD"]
    p = subprocess.Popen(cmd, cwd=gitdir, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL,
                         text=True, errors="replace", bufsize=1<<20)
    header, buf, size = None, [], 0
    for line in p.stdout:
        if line.startswith(SEP):
            if header is not None:
                yield header, "".join(buf)
            header = line[1:].rstrip("\n").split("\x00")
            buf, size = [], 0
        elif header is not None:
            if size < MAX_PATCH_BYTES * 3:
                buf.append(line); size += len(line)
    if header is not None:
        yield header, "".join(buf)
    p.stdout.close(); p.wait()

def trim_patch(patch):
    """Keep every file header; cap body lines per file. Returns (patch, nfiles, truncated)."""
    out, nfiles, truncated, kept, in_body = [], 0, False, 0, False
    for line in patch.split("\n"):
        if line.startswith("diff --git "):
            nfiles += 1; kept = 0; in_body = False
            out.append(line); continue
        if not in_body and (line.startswith(("index ","old mode","new mode","new file mode","deleted file mode",
                                             "similarity index","rename from","rename to","copy from","copy to",
                                             "--- ","+++ ","Binary files"))):
            out.append(line); continue
        in_body = True
        if kept < MAX_LINES_PER_FILE:
            out.append(line); kept += 1
        else:
            truncated = True
    text = "\n".join(out)
    if len(text) > MAX_PATCH_BYTES:
        text = text[:MAX_PATCH_BYTES]; truncated = True
    return text, nfiles, truncated

def extract(repo):
    outpath = os.path.join(RAW, repo.replace("/","__") + ".jsonl.gz")
    if os.path.exists(outpath):
        return repo, "cached"
    gitdir = clone(repo)
    n_seen = n_cc = n_kept = 0
    tmp = outpath + ".tmp"
    with gzip.open(tmp, "wt") as f:
        for header, patch in iter_commits(gitdir):
            if len(header) < 5: continue
            sha, an, ae, at, subject = header[0], header[1], header[2], header[3], header[4]
            n_seen += 1
            parsed = parse_subject(subject)
            if parsed is None: continue
            n_cc += 1
            typ, scope, breaking, subj, conv = parsed
            if not patch.strip(): continue
            text, nfiles, truncated = trim_patch(patch.lstrip("\n"))
            if nfiles == 0 or nfiles > MAX_FILES: continue
            rec = {"repo": repo, "sha": sha, "type": typ, "scope": scope, "breaking": breaking,
                   "subject": subj, "convention": conv, "bot": bool(BOT_RE.search(an+" "+ae)),
                   "ts": int(at), "nfiles": nfiles, "truncated": truncated,
                   "patch_hash": hashlib.sha1(text.encode()).hexdigest()[:16], "patch": text}
            f.write(json.dumps(rec, ensure_ascii=False) + "\n")
            n_kept += 1
    os.replace(tmp, outpath)
    if not KEEP_CLONES:
        shutil.rmtree(gitdir, ignore_errors=True)
    return repo, f"seen={n_seen} cc={n_cc} kept={n_kept}"

def stage2(repos):
    log(f"extract: {len(repos)} repos, {WORKERS} workers")
    with ThreadPoolExecutor(WORKERS) as ex:
        futs = {ex.submit(extract, r): r for r in repos}
        for fut in as_completed(futs):
            r = futs[fut]
            try:
                log("done", *fut.result())
            except Exception as e:
                log("FAIL", r, str(e)[:300])

if __name__ == "__main__":
    repos = [l.split()[0] for l in open(os.path.join(ROOT,"scripts","repos.txt")) if l.strip() and not l.startswith("#")]
    repos = list(dict.fromkeys(repos))
    only = sys.argv[1:]
    if only:
        stage2(only); sys.exit(0)
    res = stage1(repos)
    keep = [r for r in repos if "ratio" in res[r] and res[r]["ratio"] >= PROBE_MIN_RATIO and res[r]["n"] >= 30]
    log(f"keeping {len(keep)}/{len(repos)} repos")
    json.dump(keep, open(os.path.join(DATA,"repos_selected.json"),"w"), indent=1)
    stage2(keep)
    log("ALL DONE")
