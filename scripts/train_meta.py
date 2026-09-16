#!/usr/bin/env python3
"""Fit the meta-model that combines the global model with the repository's
own history, from the chronological simulation saved by
scripts/experiments/exp_local2.py, and export it for the Go CLI.

Feature layout (must match internal/history + internal/model/meta.go):
  log p_global (C) | log knn (C) | log conf (C) | log file (C) | seen | log1p(n)/8 | max_sim
"""
import struct, sys
import numpy as np
from sklearn.linear_model import LogisticRegression
from sklearn.metrics import accuracy_score

C = 11
d = np.load("data/meta_sim.npz", allow_pickle=True)
def select(F):  # drop the marginal block (ablation: useless)
    return np.hstack([F[:, :3*C], F[:, 4*C:5*C], F[:, 5*C:]])
Ftr, Ytr, Fte, Yte = select(d["Ftr"]), d["Ytr"], select(d["Fte"]), d["Yte"]
clf = LogisticRegression(max_iter=3000, C=1.0)
clf.fit(Ftr, Ytr)
P = clf.predict_proba(Fte)
top2 = np.mean([Yte[i] in np.argsort(-P[i])[:2] for i in range(len(Yte))])
print(f"held-out: top1={accuracy_score(Yte, P.argmax(1)):.4f} top2={top2:.4f}")
conf = P.max(1)
for lo, hi in [(0, .5), (.5, .7), (.7, .9), (.9, 1.01)]:
    m = (conf >= lo) & (conf < hi)
    print(f"  conf [{lo},{hi}) share={m.mean()*100:.0f}% acc={accuracy_score(Yte[m], P.argmax(1)[m]):.3f}")
if "--final" in sys.argv:
    clf.fit(np.vstack([Ftr, Fte]), np.concatenate([Ytr, Yte]))
    print("refit on all simulated rows")
W = clf.coef_.astype(np.float32)   # (C, nfeat)
b = clf.intercept_.astype(np.float32)
out = sys.argv[1] if len(sys.argv) > 1 and not sys.argv[1].startswith("--") else "internal/model/meta.bin"
with open(out, "wb") as f:
    f.write(b"CCMM"); f.write(struct.pack("<II", W.shape[1], C)); f.write(W.T.copy().tobytes()); f.write(b.tobytes())
print("exported", out, W.shape)
