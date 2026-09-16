"""Does cleaner training data help the diff-only global model?
  strict: keep only repositories whose recent history is >= 90% conventional
  clean:  drop training rows whose out-of-fold prediction confidently disagrees
          with the label (confident-learning style)
Evaluated on the same held-out repositories as everything else."""
import json, sys, time
import numpy as np
sys.path.insert(0, "scripts")
from train import load, split_by_repo, make_clf, class_weights, CLASSES, log, softmax
h, X, M, meta = load("data/features")
y = np.array([CLASSES.index(m["type"]) for m in meta])
repo = np.array([m["repo"] for m in meta])
tr, te, _ = split_by_repo(meta, 0.15, 42)
tr_idx, te_idx = np.nonzero(tr)[0], np.nonzero(te)[0]
probe = json.load(open("data/probe.json"))
ratio = np.array([probe[r]["ratio"] for r in repo])

def fit_eval(rows, name):
    clf = make_clf(1e-5, 25, class_weights(y[rows], "sqrt"), 42)
    t0 = time.time(); clf.fit(X[rows], y[rows])
    Z = clf.decision_function(X[te_idx]); pred = Z.argmax(1)
    top2 = np.mean([y[i] in np.argsort(-Z[k])[:2] for k, i in enumerate(te_idx)])
    log(f"{name:40s} rows={len(rows)} heldout top1={np.mean(pred==y[te_idx]):.4f} top2={top2:.4f} ({time.time()-t0:.0f}s)")
    return clf

fit_eval(tr_idx, "baseline (all train rows)")
strict = tr_idx[ratio[tr_idx] >= 0.9]
fit_eval(strict, "strict repos (ratio>=0.9)")
# out-of-fold predictions on train rows
train_repos = sorted(set(repo[tr_idx])); rng = np.random.RandomState(0); rng.shuffle(train_repos)
A = set(train_repos[: len(train_repos) // 2]); inA = np.array([r in A for r in repo[tr_idx]])
Poof = np.zeros((len(tr_idx), len(CLASSES)), dtype=np.float32)
for fit_mask, pred_mask in [(inA, ~inA), (~inA, inA)]:
    fi = tr_idx[fit_mask]
    clf = make_clf(1e-5, 20, class_weights(y[fi], "sqrt"), 42); clf.fit(X[fi], y[fi])
    Poof[pred_mask] = softmax(clf.decision_function(X[tr_idx[pred_mask]]) / 1.5)
np.save("data/oof_train_probs.npy", Poof)
for tau in [0.5, 0.7, 0.85]:
    disagree = (Poof.argmax(1) != y[tr_idx]) & (Poof.max(1) >= tau)
    keep = tr_idx[~disagree]
    fit_eval(keep, f"clean tau={tau} (dropped {disagree.sum()})")
disagree = (Poof.argmax(1) != y[tr_idx]) & (Poof.max(1) >= 0.7)
fit_eval(tr_idx[(~disagree) & (ratio[tr_idx] >= 0.9)], "strict + clean tau=0.7")
