"""Learned combination of the global model with the repository's own history.

1. Out-of-fold global logits for train repos (2 folds by repo), plain logits
   for held-out repos.
2. Chronological simulation on every repo: for each commit, evidence from the
   earlier commits (kNN over diffs, confusion correction, marginal, per-file
   history) -> meta features.
3. Multinomial LR meta-model fitted on train repos, evaluated on held-out.
"""
import json, sys, time
import numpy as np, scipy.sparse as sp
from collections import defaultdict
sys.path.insert(0, "scripts")
from train import load, split_by_repo, make_clf, class_weights, CLASSES, log, softmax
from sklearn.linear_model import LogisticRegression

C = len(CLASSES); T = 1.5; WARM = 20; HIST = int(sys.argv[1]) if len(sys.argv) > 1 else 1000; K = int(sys.argv[2]) if len(sys.argv) > 2 else 15
h, X, M, meta = load("data/features")
nd = h["num_dense"]
y = np.array([CLASSES.index(m["type"]) for m in meta])
repo = np.array([m["repo"] for m in meta]); ts = np.array([m["ts"] for m in meta]); sha = np.array([m["sha"] for m in meta])
tr, te, test_repos = split_by_repo(meta, 0.15, 42)
tr_idx, te_idx = np.nonzero(tr)[0], np.nonzero(te)[0]
paths = {}
for line in open("data/paths_all.jsonl"):
    r = json.loads(line); paths[r["sha"]] = r["paths"]

# --- global logits: OOF on train, plain on test
Z = np.zeros((len(y), C), dtype=np.float32)
train_repos = sorted(set(repo[tr_idx])); rng = np.random.RandomState(0); rng.shuffle(train_repos)
A = set(train_repos[: len(train_repos) // 2]); inA = np.array([r in A for r in repo[tr_idx]])
for fit_mask, pred_mask in [(inA, ~inA), (~inA, inA)]:
    fi, pi = tr_idx[fit_mask], tr_idx[pred_mask]
    clf = make_clf(1e-5, 20, class_weights(y[fi], "sqrt"), 42); clf.fit(X[fi], y[fi]); Z[pi] = clf.decision_function(X[pi])
clf = make_clf(1e-5, 25, class_weights(y[tr_idx], "sqrt"), 42); clf.fit(X[tr_idx], y[tr_idx]); Z[te_idx] = clf.decision_function(X[te_idx])
train_prior = np.bincount(y[tr_idx], minlength=C) / len(tr_idx)
log(f"global: oof-train acc={np.mean(Z[tr_idx].argmax(1)==y[tr_idx]):.4f} heldout acc={np.mean(Z[te_idx].argmax(1)==y[te_idx]):.4f}")
P = softmax(Z / T)

# --- normalised hashed vectors for similarity
Xh = X[:, nd:].tocsr().astype(np.float32)
norms = np.sqrt(np.asarray(Xh.multiply(Xh).sum(1))).ravel(); norms[norms == 0] = 1
Xh = (sp.diags(1 / norms) @ Xh).tocsr()

def norm(v):
    s = v.sum(); return v / s if s > 0 else np.full_like(v, 1 / len(v))

def simulate(rows):
    """Return meta features F (n, d), labels, repo names, and standalone kNN preds."""
    F, Y, R, KNN = [], [], [], []
    for rp in sorted(set(repo[rows])):
        idx = rows[repo[rows] == rp]
        idx = idx[np.lexsort((sha[idx], ts[idx]))]
        n = len(idx)
        Xr = Xh[idx]; S = (Xr @ Xr.T).toarray()
        counts = np.zeros(C); conf = np.zeros((C, C)); fileh = defaultdict(lambda: np.zeros(C))
        for j in range(n):
            i = idx[j]; yy = y[i]; p = P[i]; g = int(p.argmax())
            if j >= WARM:
                lo = max(0, j - HIST)
                rc = (counts + 1) / (counts.sum() + C)
                Csm = conf + 2.0 * np.eye(C) + 0.2; Csm /= Csm.sum(1, keepdims=True); pc = p @ Csm
                fp = np.zeros(C); seen = 0
                for pth in paths.get(sha[i], []):
                    if pth in fileh: fp += norm(fileh[pth]); seen += 1
                fp = (fp / seen + 0.05) / (1 + 0.05 * C) if seen else rc
                sims = S[j, lo:j]; hy = y[idx[lo:j]]
                k = min(K, len(sims)); top = np.argpartition(-sims, k - 1)[:k]
                w = np.maximum(sims[top], 0) ** 2
                kv = np.zeros(C)
                for t_, w_ in zip(hy[top], w): kv[t_] += w_
                kd = (kv + 0.3) / (kv.sum() + 0.3 * C)
                F.append(np.concatenate([np.log(p + 1e-6), np.log(kd), np.log(pc + 1e-6), np.log(rc), np.log(fp),
                                         [float(seen > 0), np.log1p(j) / 8, sims[top].max()]]))
                Y.append(yy); R.append(rp); KNN.append(kd.argmax())
            counts[yy] += 1; conf[g, yy] += 1
            for pth in paths.get(sha[i], []): fileh[pth][yy] += 1
    return np.array(F, dtype=np.float32), np.array(Y), np.array(R), np.array(KNN)

t0 = time.time()
Ftr, Ytr, Rtr, _ = simulate(tr_idx); log(f"simulated train: {Ftr.shape} in {time.time()-t0:.0f}s")
Fte, Yte, Rte, Kte = simulate(te_idx); log(f"simulated heldout: {Fte.shape}")

def report(name, pred):
    accs = [np.mean(pred[Rte == r] == Yte[Rte == r]) for r in sorted(set(Rte))]
    log(f"{name:28s} top1={np.mean(pred==Yte):.4f} macro/repo={np.mean(accs):.4f}")

report("global", Fte[:, :C].argmax(1))
report("global+prior0.7", (Fte[:, :C] + 0.7 * (Fte[:, 3*C:4*C] - np.log(train_prior))).argmax(1))
report("knn only", Kte)
report("conf only", Fte[:, 2*C:3*C].argmax(1))
for cw in [None, "balanced"]:
    for Creg in [0.1, 1.0]:
        meta_clf = LogisticRegression(max_iter=3000, C=Creg, class_weight=cw)
        meta_clf.fit(Ftr, Ytr)
        Pm = meta_clf.predict_proba(Fte)
        report(f"meta LR C={Creg} cw={cw}", Pm.argmax(1))
        top2 = np.mean([Yte[i] in np.argsort(-Pm[i])[:2] for i in range(len(Yte))])
        log(f"   top2={top2:.4f}")
# ablations of the meta model (default C=1, no cw)
blocks = {"global": slice(0, C), "knn": slice(C, 2*C), "conf": slice(2*C, 3*C), "marginal": slice(3*C, 4*C), "file": slice(4*C, 5*C)}
for name, sl in blocks.items():
    Ftr2, Fte2 = Ftr.copy(), Fte.copy(); Ftr2[:, sl] = 0; Fte2[:, sl] = 0
    mc = LogisticRegression(max_iter=3000, C=1.0); mc.fit(Ftr2, Ytr)
    report(f"meta without {name}", mc.predict(Fte2))
np.savez("data/meta_sim.npz", Ftr=Ftr, Ytr=Ytr, Rtr=Rtr, Fte=Fte, Yte=Yte, Rte=Rte)
