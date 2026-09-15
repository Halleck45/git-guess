"""Experiment: stack a gradient-boosted tree model on top of the linear model.
Inputs of the tree model: dense features + out-of-fold logits of the linear
model. Answers: is a non-linear second stage worth porting to Go?"""
import sys, time
import numpy as np, scipy.sparse as sp
sys.path.insert(0, "scripts")  # run from the repository root
from train import load, split_by_repo, with_message, make_clf, class_weights, CLASSES, log, softmax
from sklearn.ensemble import HistGradientBoostingClassifier
from sklearn.metrics import accuracy_score, f1_score

h, X, M, meta = load(sys.argv[1])
nd = h["num_dense"]
y = np.array([CLASSES.index(m["type"]) for m in meta])
tr, te, test_repos = split_by_repo(meta, 0.15, 42)
repos = np.array([m["repo"] for m in meta])
tr_idx, te_idx = np.nonzero(tr)[0], np.nonzero(te)[0]
D = X[:, :nd].toarray()

# out-of-fold logits on train (2 folds by repo)
train_repos = sorted(set(repos[tr_idx]))
rng = np.random.RandomState(0); rng.shuffle(train_repos)
foldA = set(train_repos[: len(train_repos) // 2])
inA = np.array([r in foldA for r in repos[tr_idx]])
oof = np.zeros((len(tr_idx), len(CLASSES)), dtype=np.float32)
for fit_mask, pred_mask in [(inA, ~inA), (~inA, inA)]:
    fi, pi = tr_idx[fit_mask], tr_idx[pred_mask]
    clf = make_clf(1e-5, 20, class_weights(y[fi], "sqrt"), 42)
    t0 = time.time(); clf.fit(X[fi], y[fi])
    oof[pred_mask] = clf.decision_function(X[pi])
    log(f"fold fit {time.time()-t0:.0f}s, fold acc={accuracy_score(y[pi], oof[pred_mask].argmax(1)):.4f}")
clf = make_clf(1e-5, 20, class_weights(y[tr_idx], "sqrt"), 42)
clf.fit(X[tr_idx], y[tr_idx])
Zte = clf.decision_function(X[te_idx])
log(f"LR alone test acc={accuracy_score(y[te_idx], Zte.argmax(1)):.4f}")

def feats(Dm, Z):
    P = softmax(Z)
    return np.hstack([Dm, Z, P, P.max(1, keepdims=True)])

Ftr, Fte = feats(D[tr_idx], oof), feats(D[te_idx], Zte)
for lr_, iters, leaves in [(0.1, 300, 31), (0.05, 600, 63)]:
    t0 = time.time()
    hgb = HistGradientBoostingClassifier(max_iter=iters, learning_rate=lr_, max_leaf_nodes=leaves, l2_regularization=1.0,
                                         random_state=0, early_stopping=False)
    hgb.fit(Ftr, y[tr_idx])
    pred = hgb.predict(Fte)
    log(f"STACK lr={lr_} iters={iters} leaves={leaves}: acc={accuracy_score(y[te_idx], pred):.4f} macroF1={f1_score(y[te_idx], pred, average='macro'):.4f} ({time.time()-t0:.0f}s)")
hgb = HistGradientBoostingClassifier(max_iter=300, learning_rate=0.1, max_leaf_nodes=31, random_state=0, early_stopping=False)
hgb.fit(D[tr_idx], y[tr_idx])
log(f"HGB dense-only acc={accuracy_score(y[te_idx], hgb.predict(D[te_idx])):.4f}")
