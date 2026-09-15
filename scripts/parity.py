#!/usr/bin/env python3
"""Check that the Go CLI reproduces the Python-side scoring of the exported
model on real commits: featurize (Go) -> weights (Python) vs CLI (Go end to end)."""
import gzip, json, os, struct, subprocess, sys
import numpy as np

feat_dir, model_path, raw_file, n = sys.argv[1], sys.argv[2], sys.argv[3], int(sys.argv[4])
binary = os.environ.get("BIN", "./conventional")

# --- read model
b = open(model_path, "rb").read()
assert b[:4] == b"CCM1"
fv, bits, nd, C, nS, scale = struct.unpack_from("<IIIIIf", b, 4)
off = 28
classes = []
for _ in range(C):
    ln = b[off]; off += 1; classes.append(b[off:off+ln].decode()); off += ln
bias = np.frombuffer(b, np.float32, C, off); off += 4*C
dense = np.frombuffer(b, np.float32, nd*C, off).reshape(nd, C); off += 4*nd*C
idx = np.frombuffer(b, np.uint32, nS, off); off += 4*nS
sparse = np.frombuffer(b, np.int16, nS*C, off).reshape(nS, C).astype(np.float32) * scale; off += 2*nS*C
prior = np.frombuffer(b, np.float32, C, off); off += 4*C
T = struct.unpack_from("<f", b, off)[0]
pos = {int(i): k for k, i in enumerate(idx)}

# --- features of the requested commits
h = json.load(open(os.path.join(feat_dir, "header.json")))
indptr = np.fromfile(os.path.join(feat_dir, "X.indptr"), np.int64)
indices = np.fromfile(os.path.join(feat_dir, "X.indices"), np.int32)
data = np.fromfile(os.path.join(feat_dir, "X.data"), np.float32)
meta = [json.loads(l) for l in open(os.path.join(feat_dir, "meta.jsonl"))]
by_sha = {m["sha"]: i for i, m in enumerate(meta)}

def score(row):
    z = bias.copy()
    for j, v in zip(indices[indptr[row]:indptr[row+1]], data[indptr[row]:indptr[row+1]]):
        if j < nd:
            z += v * dense[j]
        elif (j - nd) in pos:
            z += v * sparse[pos[j - nd]]
    z = z / T
    p = np.exp(z - z.max()); return p / p.sum()

checked, maxdiff, agree = 0, 0.0, 0
with gzip.open(raw_file, "rt") as f:
    for line in f:
        r = json.loads(line)
        if r["sha"] not in by_sha: continue
        p_py = score(by_sha[r["sha"]])
        out = subprocess.run([binary, "--json", "--no-prior", "--no-scope", "-n", "20", "-"], input=r["patch"], capture_output=True, text=True)
        if out.returncode != 0:
            print("CLI error", out.stderr[:200]); continue
        res = json.loads(out.stdout)
        p_go = np.array([next(c["p"] for c in res["candidates"] if c["type"] == cl) for cl in classes])
        d = np.abs(p_go - p_py).max()
        maxdiff = max(maxdiff, d)
        agree += int(p_go.argmax() == p_py.argmax())
        checked += 1
        if d > 1e-3:
            print(f"{r['sha'][:8]} diff={d:.4f} go={classes[p_go.argmax()]} py={classes[p_py.argmax()]}")
        if checked >= n: break
print(f"checked={checked} argmax_agree={agree} max_abs_prob_diff={maxdiff:.2e}")
