#!/usr/bin/env python3
"""Train the conventional commit type classifier on features produced by
`go run ./cmd/featurize` and export a compact binary model.

Usage: python3 scripts/train.py [--features data/features] [--out internal/model/model.bin]
"""
import argparse, json, os, struct, sys, time
import numpy as np
import scipy.sparse as sp
from sklearn.linear_model import SGDClassifier
from sklearn.metrics import accuracy_score, classification_report, confusion_matrix, f1_score

CLASSES = ["feat","fix","docs","style","refactor","perf","test","build","ci","chore","revert"]

def log(*a):
    print(time.strftime("%H:%M:%S"), *a, flush=True)

def load(dirpath):
    h = json.load(open(os.path.join(dirpath, "header.json")))
    ncols = h["num_dense"] + (1 << h["hash_bits"])
    def csr(prefix):
        indptr = np.fromfile(os.path.join(dirpath, prefix + ".indptr"), dtype=np.int64)
        indices = np.fromfile(os.path.join(dirpath, prefix + ".indices"), dtype=np.int32)
        data = np.fromfile(os.path.join(dirpath, prefix + ".data"), dtype=np.float32)
        return sp.csr_matrix((data, indices, indptr), shape=(len(indptr) - 1, ncols))
    X, M = csr("X"), csr("M")
    meta = [json.loads(l) for l in open(os.path.join(dirpath, "meta.jsonl"))]
    return h, X, M, meta

def with_message(X, M, has_msg_idx):
    Xm = (X + M).tocsr()
    flag = sp.csr_matrix((np.ones(X.shape[0], dtype=np.float32), (np.arange(X.shape[0]), np.full(X.shape[0], has_msg_idx))), shape=X.shape)
    return (Xm + flag).tocsr()

def split_by_repo(meta, test_frac, seed):
    repos = sorted({m["repo"] for m in meta})
    rng = np.random.RandomState(seed)
    rng.shuffle(repos)
    n_test = max(1, int(len(repos) * test_frac))
    test_repos = set(repos[:n_test])
    is_test = np.array([m["repo"] in test_repos for m in meta])
    return ~is_test, is_test, sorted(test_repos)

def topk_acc(P, y, k):
    top = np.argsort(-P, axis=1)[:, :k]
    return np.mean([y[i] in top[i] for i in range(len(y))])

def evaluate(clf, X, y, name):
    P = clf.predict_proba(X) if hasattr(clf, "predict_proba") else softmax(clf.decision_function(X))
    pred = P.argmax(1)
    acc = accuracy_score(y, pred)
    log(f"[{name}] acc={acc:.4f} top2={topk_acc(P, y, 2):.4f} top3={topk_acc(P, y, 3):.4f} macroF1={f1_score(y, pred, average='macro'):.4f}")
    return acc, P

def softmax(z):
    z = z - z.max(1, keepdims=True)
    e = np.exp(z)
    return e / e.sum(1, keepdims=True)

def make_clf(alpha, epochs, cw, seed, l1_ratio=0.0):
    penalty = "elasticnet" if l1_ratio > 0 else "l2"
    return SGDClassifier(loss="log_loss", penalty=penalty, l1_ratio=l1_ratio, alpha=alpha, max_iter=epochs, tol=1e-4,
                         class_weight=cw, random_state=seed, n_jobs=-1, average=True, early_stopping=False)

def class_weights(y, mode):
    counts = np.bincount(y, minlength=len(CLASSES)).astype(float)
    if mode == "none":
        return None
    if mode == "balanced":
        return {i: counts.sum() / (len(CLASSES) * max(c, 1)) for i, c in enumerate(counts)}
    return {i: float(np.sqrt(counts.max() / max(c, 1))) for i, c in enumerate(counts)}

def pruned_acc(clf, nd, X, y, keep_rows):
    W = clf.coef_.copy()
    Ws = W[:, nd:]
    mx = np.abs(Ws).max(axis=0)
    order = np.argsort(-mx)
    out = []
    for k in keep_rows:
        Wk = W.copy()
        drop = order[k:]
        Wk[:, nd + drop] = 0
        Z = X @ Wk.T + clf.intercept_
        out.append((k, accuracy_score(y, np.asarray(Z).argmax(1))))
    return out

def sweep(args):
    h, X, M, meta = load(args.features)
    y = np.array([CLASSES.index(m["type"]) for m in meta])
    tr, te, _ = split_by_repo(meta, args.test_frac, args.seed)
    has_msg = h["dense_names"].index("has_message")
    Xm = with_message(X, M, has_msg)
    Xtr = sp.vstack([X[tr], Xm[tr]]).tocsr()
    ytr = np.concatenate([y[tr], y[tr]])
    log(f"sweep on train={tr.sum()} test={te.sum()} cols={X.shape[1]}")
    configs = []
    for alpha in [5e-7, 1e-6, 2e-6, 5e-6, 1e-5]:
        configs.append(dict(alpha=alpha, cw="sqrt", l1=0.0))
    configs += [dict(alpha=2e-6, cw="none", l1=0.0), dict(alpha=2e-6, cw="balanced", l1=0.0),
                dict(alpha=2e-6, cw="sqrt", l1=0.15), dict(alpha=5e-6, cw="sqrt", l1=0.15)]
    results = []
    for cfg in configs:
        clf = make_clf(cfg["alpha"], args.epochs, class_weights(y[tr], cfg["cw"]), args.seed, cfg["l1"])
        t0 = time.time(); clf.fit(Xtr, ytr)
        acc, P = evaluate(clf, X[te], y[te], f"cfg={cfg}")
        accm, _ = evaluate(clf, Xm[te], y[te], f"cfg={cfg} +msg")
        nnz = int((np.abs(clf.coef_[:, h['num_dense']:]).max(axis=0) > 1e-6).sum())
        pr = pruned_acc(clf, h["num_dense"], X[te], y[te], [50000, 100000, 200000, 400000])
        f1 = f1_score(y[te], P.argmax(1), average="macro")
        results.append((cfg, acc, accm, f1, nnz, pr, time.time() - t0))
        log(f"  -> acc={acc:.4f} +msg={accm:.4f} macroF1={f1:.4f} nnz_rows={nnz} pruned={[(k, round(a,4)) for k,a in pr]} {time.time()-t0:.0f}s")
    print("\nSUMMARY")
    for cfg, acc, accm, f1, nnz, pr, dt in results:
        print(f"{cfg}  acc={acc:.4f} msg={accm:.4f} f1={f1:.4f} rows={nnz} pruned={[(k, round(a,4)) for k,a in pr]}")

def fit_temperature(Z, y):
    """Temperature scaling: minimise NLL of softmax(Z/T) over a grid."""
    best, bestT = 1e9, 1.0
    for T in np.linspace(0.5, 5.0, 91):
        P = softmax(Z / T)
        nll = -np.log(P[np.arange(len(y)), y] + 1e-12).mean()
        if nll < best:
            best, bestT = nll, T
    return float(bestT)

def export(path, clf, h, class_names, prior, keep_rows, temperature=1.0):
    """Write the binary model. See internal/model/model.go for the reader."""
    W = clf.coef_.astype(np.float32)          # (C, ncols)
    b = clf.intercept_.astype(np.float32)     # (C,)
    nd = h["num_dense"]
    C = W.shape[0]
    Wd = W[:, :nd].T.copy()                   # (nd, C)
    Ws = W[:, nd:]                            # (C, 2^bits)
    mx = np.abs(Ws).max(axis=0)
    keep = np.sort(np.argsort(-mx)[:keep_rows])
    keep = keep[mx[keep] > 0]
    Wk = Ws[:, keep].T.copy()                 # (n_keep, C)
    scale = float(np.abs(Wk).max()) / 32767.0 if len(keep) else 1.0
    Wq = np.round(Wk / scale).astype(np.int16)
    with open(path, "wb") as f:
        f.write(b"CCM1")
        f.write(struct.pack("<IIIIIf", h["feature_version"], h["hash_bits"], nd, C, len(keep), scale))
        for name in class_names:
            nb = name.encode(); f.write(struct.pack("<B", len(nb))); f.write(nb)
        f.write(b.tobytes())
        f.write(Wd.astype(np.float32).tobytes())
        f.write(keep.astype(np.uint32).tobytes())
        f.write(Wq.tobytes())
        f.write(np.asarray(prior, dtype=np.float32).tobytes())
        f.write(struct.pack("<f", temperature))
    size = os.path.getsize(path)
    log(f"exported {path}: {len(keep)} sparse rows, {size/1e6:.2f} MB")
    return keep, Wq, scale

def quantized_eval(clf, h, keep, Wq, scale, X, y, name):
    """Re-evaluate with the pruned + quantized weights to measure the loss."""
    nd = h["num_dense"]
    C = clf.coef_.shape[0]
    W = np.zeros_like(clf.coef_, dtype=np.float32)
    W[:, :nd] = clf.coef_[:, :nd]
    W[:, nd + keep] = (Wq.astype(np.float32) * scale).T
    Z = X @ W.T + clf.intercept_
    pred = np.asarray(Z).argmax(1)
    acc = accuracy_score(y, pred)
    log(f"[{name} quantized] acc={acc:.4f}")
    return acc

def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--features", default="data/features")
    ap.add_argument("--out", default="internal/model/model.bin")
    ap.add_argument("--test-frac", type=float, default=0.15)
    ap.add_argument("--seed", type=int, default=42)
    ap.add_argument("--alpha", type=float, default=2e-6)
    ap.add_argument("--epochs", type=int, default=30)
    ap.add_argument("--keep-rows", type=int, default=150000, help="hashed rows kept in the exported model (by max |w|)")
    ap.add_argument("--no-scipy", action="store_true", help="drop scipy-convention repos (numpy/pandas/...)")
    ap.add_argument("--class-weight", default="sqrt", choices=["none", "sqrt", "balanced"])
    ap.add_argument("--final", action="store_true", help="retrain on all data before export")
    ap.add_argument("--report", default="data/report.json")
    ap.add_argument("--msg-frac", type=float, default=0.5, help="fraction of training rows duplicated with their message")
    ap.add_argument("--sweep", action="store_true", help="compare several configurations and pruning levels")
    ap.add_argument("--l1-ratio", type=float, default=0.0, help=">0 switches to elasticnet")
    args = ap.parse_args()
    if args.sweep:
        return sweep(args)

    h, X, M, meta = load(args.features)
    log(f"loaded X={X.shape} nnz={X.nnz} M.nnz={M.nnz} rows={len(meta)}")
    y = np.array([CLASSES.index(m["type"]) for m in meta])
    keep_rows = np.ones(len(meta), dtype=bool)
    if args.no_scipy:
        keep_rows &= np.array([m["convention"] == "cc" for m in meta])
    X, M, y = X[keep_rows], M[keep_rows], y[keep_rows]
    meta = [m for m, k in zip(meta, keep_rows) if k]
    log("class distribution:", {CLASSES[i]: int(c) for i, c in zip(*np.unique(y, return_counts=True))})

    tr, te, test_repos = split_by_repo(meta, args.test_frac, args.seed)
    log(f"train rows={tr.sum()} test rows={te.sum()} test repos={len(test_repos)}")
    has_msg = h["dense_names"].index("has_message")
    Xte, Mte, yte = X[te], M[te], y[te]
    Xm_te = with_message(Xte, Mte, has_msg)

    # message dropout: every training commit appears without its message, and a
    # random half of them appear a second time with it (memory bound).
    rng = np.random.RandomState(args.seed)
    tr_idx = np.nonzero(tr)[0]
    aug = tr_idx[rng.rand(len(tr_idx)) < args.msg_frac]
    Xtr = sp.vstack([X[tr_idx], with_message(X[aug], M[aug], has_msg)]).tocsr()
    ytr = np.concatenate([y[tr_idx], y[aug]])
    log(f"train matrix {Xtr.shape} nnz={Xtr.nnz} ({len(aug)} message-augmented rows)")
    counts = np.bincount(y[tr], minlength=len(CLASSES)).astype(float)
    if args.class_weight == "none":
        cw = None
    elif args.class_weight == "balanced":
        cw = {i: counts.sum() / (len(CLASSES) * max(c, 1)) for i, c in enumerate(counts)}
    else:
        cw = {i: float(np.sqrt(counts.max() / max(c, 1))) for i, c in enumerate(counts)}
    log("class weights:", cw and {CLASSES[i]: round(w, 2) for i, w in cw.items()})

    clf = make_clf(args.alpha, args.epochs, cw, args.seed, args.l1_ratio)
    t0 = time.time()
    clf.fit(Xtr, ytr)
    log(f"fit in {time.time()-t0:.0f}s, iters={clf.n_iter_}")
    acc_diff, P_diff = evaluate(clf, Xte, yte, "test diff-only")
    acc_msg, P_msg = evaluate(clf, Xm_te, yte, "test diff+message")
    pred = P_diff.argmax(1)
    print(classification_report(y[te], pred, labels=list(range(len(CLASSES))), target_names=CLASSES, digits=3, zero_division=0))
    cm = confusion_matrix(y[te], pred, labels=list(range(len(CLASSES))))
    print("confusion (rows=true, cols=pred):\n", "      " + " ".join(f"{c[:5]:>5}" for c in CLASSES))
    for i, row in enumerate(cm):
        print(f"{CLASSES[i][:5]:>5} " + " ".join(f"{v:5d}" for v in row))
    # confidence calibration buckets
    conf = P_diff.max(1)
    for lo, hi in [(0, .4), (.4, .6), (.6, .8), (.8, 1.01)]:
        m = (conf >= lo) & (conf < hi)
        if m.sum():
            log(f"conf [{lo:.1f},{hi:.1f}) n={m.sum()} ({m.mean()*100:.0f}%) acc={accuracy_score(y[te][m], pred[m]):.3f}")
    # per-repo accuracy on held-out repos
    per_repo = {}
    for i, m in enumerate(np.array(meta, dtype=object)[te]):
        per_repo.setdefault(m["repo"], []).append(pred[i] == y[te][i])
    worst = sorted(((np.mean(v), k, len(v)) for k, v in per_repo.items()))[:8]
    log("worst held-out repos:", [(k, round(a, 3), n) for a, k, n in worst])

    Zte = clf.decision_function(Xte)
    T = fit_temperature(Zte, y[te])
    Pt = softmax(Zte / T)
    log(f"temperature={T:.2f}")
    conf = Pt.max(1)
    for lo, hi in [(0, .4), (.4, .6), (.6, .8), (.8, 1.01)]:
        m = (conf >= lo) & (conf < hi)
        if m.sum():
            log(f"calibrated conf [{lo:.1f},{hi:.1f}) n={m.sum()} ({m.mean()*100:.0f}%) acc={accuracy_score(y[te][m], pred[m]):.3f}")
    prior = np.bincount(y[tr], minlength=len(CLASSES)) / tr.sum()
    # Simulate the CLI's repository-prior adaptation on held-out repos: each
    # test commit is re-weighted with its own repository's type distribution
    # (what `git log` gives at inference time).
    te_meta = [m for m, k in zip(meta, te) if k]
    repo_counts = {}
    for m, yy in zip(te_meta, yte):
        repo_counts.setdefault(m["repo"], np.zeros(len(CLASSES)))[yy] += 1
    Pm_t = softmax(clf.decision_function(Xm_te) / T)
    for lam in [0.0, 0.3, 0.5, 0.7, 1.0]:
        accs = []
        for P in (Pt, Pm_t):
            Pa = P.copy()
            for i, m in enumerate(te_meta):
                rc = (repo_counts[m["repo"]] + 1) / (repo_counts[m["repo"]].sum() + len(CLASSES))
                Pa[i] *= (rc / np.maximum(prior, 1e-4)) ** lam
            accs.append(accuracy_score(yte, Pa.argmax(1)))
        log(f"repo-prior lambda={lam}: diff-only acc={accs[0]:.4f}  +msg acc={accs[1]:.4f}")
    if args.final:
        log("retraining on all rows for export")
        del Xtr
        aug = np.nonzero(rng.rand(len(y)) < args.msg_frac)[0]
        Xall = sp.vstack([X, with_message(X[aug], M[aug], has_msg)]).tocsr()
        yall = np.concatenate([y, y[aug]])
        clf.fit(Xall, yall)
        del Xall
        prior = np.bincount(y, minlength=len(CLASSES)) / len(y)
    for k, a in pruned_acc(clf, h["num_dense"], Xte, yte, [50000, 100000, 150000, 300000]):
        log(f"pruned to {k} rows: diff-only acc={a:.4f}")
    keep, Wq, scale = export(args.out, clf, h, CLASSES, prior, args.keep_rows, T)
    qacc = quantized_eval(clf, h, keep, Wq, scale, Xte, yte, "test diff-only")
    json.dump({"acc_diff": acc_diff, "acc_msg": acc_msg, "acc_quantized": qacc, "test_repos": test_repos, "temperature": T,
               "rows": int(len(y)), "sparse_rows": int(len(keep)), "alpha": args.alpha, "class_weight": args.class_weight},
              open(args.report, "w"), indent=1)

if __name__ == "__main__":
    main()
