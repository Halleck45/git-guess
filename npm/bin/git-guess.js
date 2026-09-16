#!/usr/bin/env node
'use strict';
const { spawnSync } = require('child_process');
const { ensureBinary } = require('../lib/download');

ensureBinary()
  .then((bin) => {
    const r = spawnSync(bin, process.argv.slice(2), { stdio: 'inherit' });
    if (r.error) throw r.error;
    process.exit(r.status === null ? 1 : r.status);
  })
  .catch((e) => {
    process.stderr.write(`git-guess: ${e.message}\n`);
    process.exit(1);
  });
