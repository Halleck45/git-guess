"""Fit the global model on the train split and save logits + row info for
adaptation experiments (chronological, per held-out repository)."""
import sys, json, time
import numpy as np, scipy.sparse as sp
sys.path.insert(0, "scripts")
from train import load, split_by_repo, make_clf, class_weights, CLASSES, log
h, X, M, meta = load("data/features")
y = np.array([CLASSES.index(m["type"]) for m in meta])
tr, te, test_repos = split_by_repo(meta, 0.15, 42)
tr_idx, te_idx = np.nonzero(tr)[0], np.nonzero(te)[0]
clf = make_clf(1e-5, 25, class_weights(y[tr_idx], "sqrt"), 42)
t0 = time.time(); clf.fit(X[tr_idx], y[tr_idx]); log(f"fit {time.time()-t0:.0f}s")
Z = clf.decision_function(X[te_idx]).astype(np.float32)
log(f"held-out acc={np.mean(Z.argmax(1)==y[te_idx]):.4f}")
prior = np.bincount(y[tr_idx], minlength=len(CLASSES)) / len(tr_idx)
np.savez("data/heldout.npz", Z=Z, y=y[te_idx], idx=te_idx, prior=prior,
         repo=np.array([meta[i]["repo"] for i in te_idx]), ts=np.array([meta[i]["ts"] for i in te_idx]),
         sha=np.array([meta[i]["sha"] for i in te_idx]))
# L2-normalised held-out feature rows for kNN (dense part dropped)
Xte = X[te_idx][:, h["num_dense"]:].tocsr().astype(np.float32)
norms = np.sqrt(np.asarray(Xte.multiply(Xte).sum(1))).ravel(); norms[norms == 0] = 1
Xte = sp.diags(1 / norms) @ Xte
sp.save_npz("data/heldout_X.npz", Xte.tocsr())
json.dump(test_repos, open("data/heldout_repos.json", "w"))
log("saved")
