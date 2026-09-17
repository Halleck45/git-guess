#!/usr/bin/env python3
"""Replay a gitmoji repository and compare the guessed emoji with the author's.

    go build -o /tmp/gg ./cmd/git-guess
    curl -fsSLo /tmp/gitmojis.json https://raw.githubusercontent.com/carloscuesta/gitmoji/master/packages/gitmojis/src/gitmojis.json
    python3 scripts/eval_gitmoji.py /tmp/gg /path/to/repo 600 /tmp/gitmojis.json

Each commit is guessed from its diff and its subject without the emoji, as a
user would type it. The local history seen by the guess includes commits
newer than the one replayed, so the numbers are slightly optimistic.
"""
import subprocess, json, sys, re, collections
GG = sys.argv[1]; repo = sys.argv[2]; n = int(sys.argv[3]); table = sys.argv[4] if len(sys.argv) > 4 else "gitmojis.json"
log = subprocess.run(["git","-C",repo,"log","--no-merges","--format=%H%x00%s",f"-n{n}"],capture_output=True,text=True).stdout
G=json.load(open(table))["gitmojis"]
bycode={g["code"]:g["emoji"].replace("\ufe0f","") for g in G}
unis=sorted({g["emoji"].replace("\ufe0f","") for g in G}, key=len, reverse=True)
def head(subj):
    s=subj.replace("\ufe0f","").lstrip()
    if s.startswith(":"):
        e=s.find(":",1)
        if e>0 and s[:e+1] in bycode: return bycode[s[:e+1]], s[e+1:].lstrip()
        return None, None
    for u in unis:
        if s.startswith(u): return u, s[len(u):].lstrip()
    return None, None
rows=[]
for line in log.splitlines():
    sha, subj = line.split("\x00",1)
    actual, rest = head(subj)
    if actual is None: continue
    rest = re.sub(r"^\([^)]*\):?\s*", "", rest)
    r = subprocess.run([GG,"--gitmoji=code","--json","-m",rest,sha],cwd=repo,capture_output=True,text=True)
    if r.returncode!=0: continue
    j=json.loads(r.stdout)
    rows.append((sha[:7], actual.replace("️",""), j["emoji"].replace("️",""), j["emoji_code"], j["type"], rest[:60], j["files"]))
tot=ok=0; miss=collections.Counter(); bot=collections.Counter()
for sha,a,p,code,typ,rest,files in rows:
    dep = rest.lower().startswith("bump ") or rest.lower().startswith("update dependency") or "renovate" in rest.lower()
    if a.startswith(":"): continue
    tot+=1
    hit = (a==p)
    if hit: ok+=1
    else: miss[(a,p)]+=1
    bot[("dep" if dep else "human", hit)]+=1
print(f"{repo}: {ok}/{tot} exact emoji = {ok/tot:.0%}")
for k,v in sorted(bot.items()): print("  ",k,v)
print("top confusions (actual -> predicted):")
for (a,p),v in miss.most_common(14): print(f"   {v:3d}  {a} -> {p}")
print("sample misses:")
shown=0
for sha,a,p,code,typ,rest,files in rows:
    if a!=p and not a.startswith(":"):
        print(f"   {sha} {a} -> {p} ({typ}, {files} files)  {rest}"); shown+=1
        if shown>=25: break
print("per (type, predicted) -> actual distribution, refinements only:")
byp=collections.defaultdict(collections.Counter)
for sha,a,p,code,typ,rest,files in rows: byp[(typ,p)][a]+=1
for (typ,p),c in sorted(byp.items(), key=lambda kv:-sum(kv[1].values())):
    if p in "💄🔒🔖⬆🔨👷🚨🌐🔥🚚🏷🗃💡🔊🔇📄🙈🍱📸🤡✏🚑♿🥅🗑🏗📌➕➖⬇📦🎨":
        print(f"   {typ:9s} {p} n={sum(c.values()):3d}  ", "  ".join(f"{k}{v}" for k,v in c.most_common(6)))
