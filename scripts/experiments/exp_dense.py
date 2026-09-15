"""Experiment: how much do the dense structural features alone give, with a
tree model (HistGradientBoosting) vs logistic regression?"""
import sys, json, os, time
import numpy as np, scipy.sparse as sp
sys.path.insert(0, "scripts")  # run from the repository root
from train import load, split_by_repo, CLASSES, log
from sklearn.ensemble import HistGradientBoostingClassifier
from sklearn.linear_model import LogisticRegression
from sklearn.metrics import accuracy_score

h, X, M, meta = load(sys.argv[1])
nd = h["num_dense"]
y = np.array([CLASSES.index(m["type"]) for m in meta])
tr, te, test_repos = split_by_repo(meta, 0.15, 42)
D = X[:, :nd].toarray()
log("test repos:", test_repos)
t0 = time.time()
hgb = HistGradientBoostingClassifier(max_iter=300, learning_rate=0.1, max_leaf_nodes=31, random_state=0)
hgb.fit(D[tr], y[tr])
log(f"HGB dense-only acc={accuracy_score(y[te], hgb.predict(D[te])):.4f} ({time.time()-t0:.0f}s)")
lr = LogisticRegression(max_iter=2000, C=10)
lr.fit(D[tr], y[tr])
log(f"LR dense-only acc={accuracy_score(y[te], lr.predict(D[te])):.4f}")
