'use strict';
// Best effort: warm the cache so the first run is instant. Any failure
// (offline, restricted CI) is silent; bin/git-guess.js downloads on demand.
require('./download').ensureBinary().catch(() => {});
