"""Chronological evaluation of local adaptation strategies on held-out
repositories: at each commit, only the repository's earlier commits are
available (as `git log` would give at inference time).

Strategies:
  base      global model only
  prior     marginal type distribution of the history (current CLI, lambda 0.7)
  conf      confusion correction: P(true | global guess) estimated on history
  file      per-file history: types of past commits touching the same files
  knn       nearest neighbours among history diffs (cosine on hashed features)
  combos
"""
import json, sys
import numpy as np, scipy.sparse as sp
from collections import defaultdict

T = 1.5
WARM = 30          # commits of history before scoring starts
HIST = 300         # history window
C = 11
CLASSES = ["feat","fix","docs","style","refactor","perf","test","build","ci","chore","revert"]

d = np.load("data/heldout.npz", allow_pickle=True)
Z, y, repo, ts, sha, train_prior = d["Z"], d["y"], d["repo"], d["ts"], d["sha"], d["prior"]
X = sp.load_npz("data/heldout_X.npz").tocsr()
paths = {}
for line in open("data/heldout_paths.jsonl"):
    r = json.loads(line); paths[r["sha"]] = r["paths"]

def softmax(z):
    z = z - z.max(-1, keepdims=True); e = np.exp(z); return e / e.sum(-1, keepdims=True)

P = softmax(Z / T)
results = defaultdict(list)   # strategy -> list of (correct, top2)
per_repo = defaultdict(lambda: defaultdict(list))

def record(name, p, yy, rp):
    order = np.argsort(-p)
    results[name].append((order[0] == yy, yy in order[:2]))
    per_repo[rp][name].append(order[0] == yy)

def norm(v):
    s = v.sum(); return v / s if s > 0 else np.full_like(v, 1 / len(v))

for rp in sorted(set(repo)):
    idx = np.nonzero(repo == rp)[0]
    idx = idx[np.lexsort((sha[idx], ts[idx]))]        # chronological
    n = len(idx)
    Xr = X[idx]
    S = (Xr @ Xr.T).toarray().astype(np.float32)      # cosine similarities
    counts = np.zeros(C)                               # marginal history
    conf = np.zeros((C, C))                            # conf[guess, true]
    fileh = defaultdict(lambda: np.zeros(C))           # path -> type counts
    for j in range(n):
        i = idx[j]; yy = y[i]; p = P[i]; g = int(p.argmax())
        lo = max(0, j - HIST)
        if j >= WARM:
            record("base", p, yy, rp)
            # marginal prior
            rc = (counts + 1) / (counts.sum() + C)
            record("prior", p * (rc / np.maximum(train_prior, 1e-4)) ** 0.7, yy, rp)
            # confusion correction: p'(t) = sum_g p(g) * P(t | guess g)
            Csm = conf + 2.0 * np.eye(C) + 0.2         # smoothing toward identity
            Csm = Csm / Csm.sum(1, keepdims=True)
            pc = p @ Csm
            record("conf", pc, yy, rp)
            # file history
            fp = np.zeros(C); seen = 0
            for pth in paths.get(sha[i], []):
                if pth in fileh:
                    fp += norm(fileh[pth]); seen += 1
            if seen:
                fp = (fp / seen + 0.05) / (1 + 0.05 * C)
                pf = p * (fp / np.maximum(train_prior, 1e-4)) ** 0.7
            else:
                pf = p * (rc / np.maximum(train_prior, 1e-4)) ** 0.7
            record("file", pf, yy, rp)
            # kNN over history diffs
            sims = S[j, lo:j]; hy = y[idx[lo:j]]
            k = min(15, len(sims)); top = np.argpartition(-sims, k - 1)[:k]
            w = np.maximum(sims[top], 0) ** 2
            kv = np.zeros(C)
            for t_, w_ in zip(hy[top], w): kv[t_] += w_
            kd = (kv + 0.3) / (kv.sum() + 0.3 * C)
            record("knn_only", kd, yy, rp)
            record("knn", p * (kd / np.maximum(train_prior, 1e-4)) ** 0.7, yy, rp)
            # combos
            record("conf+file", pc * (fp / np.maximum(train_prior, 1e-4)) ** 0.7 if seen else pc * (rc / np.maximum(train_prior, 1e-4)) ** 0.7, yy, rp)
            record("conf+knn", pc * (kd / np.maximum(train_prior, 1e-4)) ** 0.7, yy, rp)
            base_c = pc * (kd / np.maximum(train_prior, 1e-4)) ** 0.5
            record("conf+knn+file", base_c * ((fp if seen else rc) / np.maximum(train_prior, 1e-4)) ** 0.5, yy, rp)
        # update history with the true label (as git log would show later)
        counts[yy] += 1; conf[g, yy] += 1
        for pth in paths.get(sha[i], []): fileh[pth][yy] += 1

print(f"{'strategy':14s} {'top1':>6s} {'top2':>6s} {'macro/repo':>10s}   n={len(results['base'])}")
for name, rows in results.items():
    a = np.array(rows)
    mr = np.mean([np.mean(per_repo[r][name]) for r in per_repo])
    print(f"{name:14s} {a[:,0].mean():6.3f} {a[:,1].mean():6.3f} {mr:10.3f}")
print("\nper repo (base -> conf+knn+file):")
for r in sorted(per_repo, key=lambda r: np.mean(per_repo[r]['base'])):
    print(f"  {r:35s} {np.mean(per_repo[r]['base']):.3f} -> {np.mean(per_repo[r]['conf+knn+file']):.3f}  (n={len(per_repo[r]['base'])})")
